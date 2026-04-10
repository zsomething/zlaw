package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/chickenzord/zlaw/internal/session"
)

const editThrottle = 1500 * time.Millisecond

// chatSink is an OutputSink for a specific Telegram chat.
// It handles streaming by accumulating deltas and throttling message edits
// to avoid hitting Telegram's rate limit (~1 edit/second per message).
// The sink is persistent for the lifetime of the session.
type chatSink struct {
	bot    *Bot
	chatID int64
	logger *slog.Logger

	mu       sync.Mutex
	msgID    int            // ID of the in-progress bot message; 0 = none
	buf      strings.Builder
	lastEdit time.Time
}

func newChatSink(bot *Bot, chatID int64, logger *slog.Logger) *chatSink {
	return &chatSink{
		bot:    bot,
		chatID: chatID,
		logger: logger,
	}
}

// Capabilities reports that this sink supports both streaming and the typing indicator.
func (s *chatSink) Capabilities() session.ChannelCaps {
	return session.ChannelCaps{Streaming: true, TypingIndicator: true}
}

// SendTyping sends a "typing" chat action to Telegram.
func (s *chatSink) SendTyping(ctx context.Context) error {
	return s.bot.SendChatAction(ctx, s.chatID, "typing")
}

// Send handles incoming session events.
// EventAssistantDelta: buffer the delta and edit the in-progress message
// (throttled to editThrottle).
// EventAssistantDone: perform a final edit with the complete text (prepending a
// quote of the original input when the turn originated from a non-Telegram channel),
// then reset state for the next turn.
// Other event types are silently ignored.
func (s *chatSink) Send(ctx context.Context, e session.Event) error {
	switch e.Type {
	case session.EventAssistantDelta:
		return s.handleDelta(ctx, e.Data)
	case session.EventAssistantDone:
		return s.handleDone(ctx, e)
	case session.EventError:
		_, err := s.bot.SendMessage(ctx, s.chatID, "⚠️ "+e.Data)
		return err
	}
	return nil
}

// Close is a no-op; Telegram chat connections are stateless from the bot side.
func (s *chatSink) Close() error { return nil }

func (s *chatSink) handleDelta(ctx context.Context, delta string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf.WriteString(delta)
	text := mdToHTML(s.buf.String())

	if s.msgID == 0 {
		// First delta: send the initial message to get a message ID.
		msgID, err := s.bot.SendMessage(ctx, s.chatID, text)
		if err != nil {
			s.logger.Warn("telegram: failed to send initial message", "chat_id", s.chatID, "error", err)
			return nil // don't propagate; keep receiving deltas
		}
		s.msgID = msgID
		s.lastEdit = time.Now()
		return nil
	}

	// Throttle edits to avoid Telegram rate limits.
	if time.Since(s.lastEdit) < editThrottle {
		return nil
	}

	if err := s.bot.EditMessageText(ctx, s.chatID, s.msgID, text); err != nil {
		if !errors.Is(err, ErrMsgNotModified) {
			s.logger.Warn("telegram: edit message failed", "chat_id", s.chatID, "error", err)
		}
	}
	s.lastEdit = time.Now()
	return nil
}

func (s *chatSink) handleDone(ctx context.Context, e session.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	defer func() {
		s.msgID = 0
		s.buf.Reset()
	}()

	rawText := e.Data
	if rawText == "" {
		rawText = s.buf.String()
	}
	if rawText == "" {
		return nil // nothing to send
	}

	// Prepend a quote of the original input when the turn came from outside Telegram
	// (e.g. CLI attach), so the Telegram user sees what triggered the response.
	if e.Origin != "" && e.Origin != "telegram" && e.Input != "" {
		rawText = "📎 " + e.Input + "\n\n" + rawText
	}

	finalText := mdToHTML(rawText)

	if s.msgID == 0 {
		// No in-progress message (non-streaming path): send the full response.
		_, err := s.bot.SendMessage(ctx, s.chatID, finalText)
		return err
	}

	// Final edit to ensure the complete text is visible.
	if err := s.bot.EditMessageText(ctx, s.chatID, s.msgID, finalText); err != nil {
		if errors.Is(err, ErrMsgNotModified) {
			return nil // already up-to-date
		}
		// Edit failed; try sending a new message as fallback.
		s.logger.Warn("telegram: final edit failed, sending new message", "chat_id", s.chatID, "error", err)
		_, err = s.bot.SendMessage(ctx, s.chatID, finalText)
		return err
	}
	return nil
}

// compile-time check.
var _ session.OutputSink = (*chatSink)(nil)
