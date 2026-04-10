package telegram

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/chickenzord/zlaw/internal/session"
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
	token   string
	logger  *slog.Logger

	// mu protects sinks to avoid creating duplicate chatSinks for the same chat.
	mu    sync.Mutex
	sinks map[int64]*chatSink
}

// NewAdapter creates an Adapter.
func NewAdapter(token string, manager *session.Manager, logger *slog.Logger) *Adapter {
	return &Adapter{
		bot:     NewBot(token),
		manager: manager,
		token:   token,
		logger:  logger,
		sinks:   make(map[int64]*chatSink),
	}
}

// Run starts the long-polling loop. Blocks until ctx is cancelled.
func (a *Adapter) Run(ctx context.Context) error {
	a.logger.Info("telegram: adapter started")
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

// handleMessage routes a Telegram message to the session manager.
func (a *Adapter) handleMessage(ctx context.Context, msg *TGMsg) {
	chatID := msg.Chat.ID
	sid := sessionIDForChat(a.token, chatID)

	a.logger.Info("telegram: message received",
		"chat_id", chatID,
		"session_id", sid,
		"text_len", len(msg.Text),
	)

	// Ensure a chatSink exists for this chat and is registered with the session.
	a.mu.Lock()
	sink, exists := a.sinks[chatID]
	if !exists {
		sink = newChatSink(a.bot, chatID, a.logger)
		a.sinks[chatID] = sink
		// GetOrCreate before Submit ensures the sink is in the broadcaster
		// before the session goroutine can pick up the queued turn.
		s := a.manager.GetOrCreate(ctx, sid)
		s.Broadcaster.Add(sink)
	}
	a.mu.Unlock()

	a.manager.Submit(ctx, sid, msg.Text)
}
