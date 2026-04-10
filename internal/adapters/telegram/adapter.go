package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chickenzord/zlaw/internal/session"
	"github.com/chickenzord/zlaw/internal/slashcmd"
)

// sessionIDForChat derives a stable session ID from the bot token and chat ID.
// Format: first 16 hex characters of sha256(token + ":" + chatID).
func sessionIDForChat(token string, chatID int64) string {
	h := sha256.Sum256([]byte(token + ":" + strconv.FormatInt(chatID, 10)))
	return fmt.Sprintf("%x", h[:8])
}

// Adapter runs the Telegram long-polling loop and routes messages to sessions.
type Adapter struct {
	bot     *Bot
	manager *session.Manager
	cmds    *slashcmd.Registry
	history slashcmd.HistoryManager // optional; backs /clear and /history
	token   string
	logger  *slog.Logger

	// mu protects sinks and sessionOverrides.
	mu sync.Mutex
	// sinks holds one chatSink per chat ID.
	sinks map[int64]*chatSink
	// sessionOverrides holds per-chat session ID overrides set after /clear.
	// When present, the override replaces the deterministic session ID so that
	// the next message starts a new session. The old session files are preserved.
	sessionOverrides map[int64]string
}

// NewAdapter creates an Adapter.
func NewAdapter(token string, manager *session.Manager, logger *slog.Logger) *Adapter {
	r := slashcmd.New()
	slashcmd.RegisterBuiltins(r)
	return &Adapter{
		bot:              NewBot(token),
		manager:          manager,
		cmds:             r,
		token:            token,
		logger:           logger,
		sinks:            make(map[int64]*chatSink),
		sessionOverrides: make(map[int64]string),
	}
}

// activeSessionID returns the current session ID for chatID.
// After /clear, this returns the override session ID until the next clear.
func (a *Adapter) activeSessionID(chatID int64) string {
	if override, ok := a.sessionOverrides[chatID]; ok {
		return override
	}
	return sessionIDForChat(a.token, chatID)
}

// rotateSessionID generates a new random session ID for chatID and stores it
// as the override. Called on /clear so subsequent messages start a new session.
func (a *Adapter) rotateSessionID(chatID int64) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback: use a hash of the old ID + timestamp.
		h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", chatID, time.Now().UnixNano())))
		copy(b[:], h[:8])
	}
	newSID := fmt.Sprintf("%x", b)
	a.sessionOverrides[chatID] = newSID
	return newSID
}

// SetHistoryManager attaches a HistoryManager that backs the /clear and
// /history slash commands. Without it those commands return an error.
func (a *Adapter) SetHistoryManager(h slashcmd.HistoryManager) {
	a.history = h
}

// Commands returns the slash command registry, allowing callers to register
// additional commands before the adapter starts.
func (a *Adapter) Commands() *slashcmd.Registry {
	return a.cmds
}

// syncCommands registers the bot's command list with Telegram via setMyCommands.
// Called once at startup so the Telegram UI shows the command picker.
func (a *Adapter) syncCommands(ctx context.Context) {
	var tgCmds []BotCommand
	for _, cmd := range a.cmds.All() {
		desc := cmd.Description
		// Telegram max description length is 256 chars.
		if len(desc) > 256 {
			desc = desc[:253] + "..."
		}
		tgCmds = append(tgCmds, BotCommand{
			Command:     cmd.Name,
			Description: desc,
		})
	}
	if err := a.bot.SetMyCommands(ctx, tgCmds); err != nil {
		a.logger.Warn("telegram: setMyCommands failed", "error", err)
	} else {
		a.logger.Info("telegram: commands synced", "count", len(tgCmds))
	}
}

// Run starts the long-polling loop. Blocks until ctx is cancelled.
func (a *Adapter) Run(ctx context.Context) error {
	a.logger.Info("telegram: adapter started")
	a.syncCommands(ctx)
	offset := 0
	for {
		if ctx.Err() != nil {
			return nil
		}

		updates, err := a.bot.GetUpdates(ctx, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			a.logger.Error("telegram: getUpdates failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			a.handleMessage(ctx, u.Message)
		}
	}
}

// handleMessage routes a Telegram message to the session manager, or handles
// it as a slash command if the text starts with "/".
func (a *Adapter) handleMessage(ctx context.Context, msg *TGMsg) {
	chatID := msg.Chat.ID
	text := msg.Text

	a.mu.Lock()
	sid := a.activeSessionID(chatID)
	a.mu.Unlock()

	a.logger.Info("telegram: message received",
		"chat_id", chatID,
		"session_id", sid,
		"text_len", len(text),
	)

	// Intercept slash commands before they reach the agent.
	// Telegram appends "@botname" to commands in group chats; strip it.
	if strings.HasPrefix(text, "/") {
		cmdText := text
		if idx := strings.Index(cmdText, "@"); idx != -1 {
			cmdText = cmdText[:idx] + cmdText[strings.IndexByte(cmdText[idx:], ' ')+idx+1:]
			cmdText = strings.TrimSpace(cmdText)
		}
		env := slashcmd.Env{
			SessionID: sid,
			History:   a.history,
			AfterClear: func(_ string) {
				// Rotate to a new session ID so the next message starts fresh.
				// The old session files are preserved on disk.
				a.mu.Lock()
				newSID := a.rotateSessionID(chatID)
				// Drop the stale sink so the next message registers a new one
				// against the new session.
				delete(a.sinks, chatID)
				a.mu.Unlock()
				a.logger.Info("telegram: session rotated after /clear",
					"chat_id", chatID, "new_session_id", newSID)
			},
		}
		resp, matched := a.cmds.Dispatch(ctx, cmdText, env)
		if matched {
			// ActionExit is not meaningful on Telegram — treat as a no-op.
			if resp.Text != "" {
				if _, err := a.bot.SendMessage(ctx, chatID, resp.Text); err != nil {
					a.logger.Error("telegram: send slash command response failed",
						"chat_id", chatID, "error", err)
				}
			}
			return
		}
		// Unknown slash command already returns a helpful error from the registry.
	}

	// Ensure a chatSink exists for this chat and is registered with the session.
	a.mu.Lock()
	// Re-read sid under lock in case /clear just rotated it above.
	sid = a.activeSessionID(chatID)
	sink, exists := a.sinks[chatID]
	if !exists {
		sink = newChatSink(a.bot, chatID, a.logger)
		a.sinks[chatID] = sink
		s := a.manager.GetOrCreate(ctx, sid)
		s.Broadcaster.Add(sink)
	}
	a.mu.Unlock()

	a.manager.Submit(ctx, sid, text, "telegram")
}
