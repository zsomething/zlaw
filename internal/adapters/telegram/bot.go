// Package telegram implements a Telegram Bot adapter using the Telegram Bot API
// over raw HTTP (no external SDK dependency).
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const apiBase = "https://api.telegram.org/bot"

// ErrMsgNotModified is returned by EditMessageText when Telegram rejects the
// request because the text is identical to the current message content.
var ErrMsgNotModified = errors.New("telegram: message is not modified")

// Update is a Telegram update from getUpdates.
type Update struct {
	UpdateID int    `json:"update_id"`
	Message  *TGMsg `json:"message,omitempty"`
}

// TGMsg is a Telegram message.
type TGMsg struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// Chat is a Telegram chat object.
type Chat struct {
	ID int64 `json:"id"`
}

// Bot is a lightweight Telegram Bot API client using raw HTTP.
type Bot struct {
	token  string
	client *http.Client
}

// NewBot creates a Bot with the given token.
// The HTTP client timeout is set to 65 seconds to accommodate 30-second
// long-polling requests with margin.
func NewBot(token string) *Bot {
	return &Bot{
		token: token,
		client: &http.Client{
			Timeout: 65 * time.Second,
		},
	}
}

// apiURL returns the full URL for a Bot API method.
func (b *Bot) apiURL(method string) string {
	return apiBase + b.token + "/" + method
}

// post sends a JSON POST request to the given Bot API method and returns the
// decoded result field.
func (b *Bot) post(ctx context.Context, method string, body any) (json.RawMessage, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("telegram: marshal %s body: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL(method), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("telegram: new request %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: %s: %w", method, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var apiResp struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("telegram: decode %s response: %w", method, err)
	}
	if !apiResp.OK {
		if apiResp.Description == "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message" {
			return nil, ErrMsgNotModified
		}
		return nil, fmt.Errorf("telegram: %s: %s", method, apiResp.Description)
	}
	return apiResp.Result, nil
}

// GetUpdates fetches pending updates using long polling.
// offset is the update_id of the first update to return (exclusive).
// timeout is the long-polling timeout in seconds.
func (b *Bot) GetUpdates(ctx context.Context, offset, timeout int) ([]Update, error) {
	raw, err := b.post(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeout,
		"allowed_updates": []string{"message"},
	})
	if err != nil {
		return nil, err
	}
	var updates []Update
	if err := json.Unmarshal(raw, &updates); err != nil {
		return nil, fmt.Errorf("telegram: decode updates: %w", err)
	}
	return updates, nil
}

// SendMessage sends a new text message to chatID and returns the sent message ID.
func (b *Bot) SendMessage(ctx context.Context, chatID int64, text string) (int, error) {
	raw, err := b.post(ctx, "sendMessage", map[string]any{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"text":       text,
		"parse_mode": "HTML",
	})
	if err != nil {
		return 0, err
	}
	var msg TGMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return 0, fmt.Errorf("telegram: decode sendMessage result: %w", err)
	}
	return msg.MessageID, nil
}

// EditMessageText replaces the text of an existing message.
// Returns ErrMsgNotModified when the text is identical to the current content.
func (b *Bot) EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error {
	_, err := b.post(ctx, "editMessageText", map[string]any{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	})
	return err
}

// SendChatAction sends a chat action (e.g. "typing") to chatID.
func (b *Bot) SendChatAction(ctx context.Context, chatID int64, action string) error {
	_, err := b.post(ctx, "sendChatAction", map[string]any{
		"chat_id": strconv.FormatInt(chatID, 10),
		"action":  action,
	})
	return err
}
