package security

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const telegramAPIBase = "https://api.telegram.org/bot"

// TelegramApprover implements Approver using the Telegram Bot API.
type TelegramApprover struct {
	botToken string
	chatID   string
	client   *http.Client
}

// NewTelegramApprover creates a TelegramApprover with the given bot token and chat ID.
func NewTelegramApprover(botToken, chatID string) *TelegramApprover {
	return &TelegramApprover{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 35 * time.Second},
	}
}

// RequestApproval sends a Telegram message with Approve/Deny buttons and polls for a response.
// It blocks until the user responds or ctx is cancelled.
func (t *TelegramApprover) RequestApproval(ctx context.Context, action string) (bool, error) {
	uuid, err := generateUUID()
	if err != nil {
		return false, fmt.Errorf("generate approval uuid: %w", err)
	}

	msgID, err := t.sendApprovalMessage(action, uuid)
	if err != nil {
		return false, fmt.Errorf("send approval message: %w", err)
	}

	approved, err := t.pollForCallback(ctx, uuid)
	if err != nil {
		_ = t.editMessage(msgID, fmt.Sprintf("⏰ Expired: %s", action))
		return false, err
	}

	status := "✅ Approved"
	if !approved {
		status = "❌ Denied"
	}
	_ = t.editMessage(msgID, fmt.Sprintf("%s: %s", status, action))
	return approved, nil
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// sendApprovalMessage posts a message with inline Approve/Deny keyboard buttons.
// Returns the message_id of the sent message.
func (t *TelegramApprover) sendApprovalMessage(action, uuid string) (int64, error) {
	payload := map[string]any{
		"chat_id": t.chatID,
		"text":    fmt.Sprintf("🔐 Approval required\n\nAction: %s", action),
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]string{
				{
					{"text": "✅ Approve", "callback_data": "approve:" + uuid},
					{"text": "❌ Deny", "callback_data": "deny:" + uuid},
				},
			},
		},
	}

	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
		Description string `json:"description"`
	}

	if err := t.post("sendMessage", payload, &resp); err != nil {
		return 0, err
	}
	if !resp.OK {
		return 0, fmt.Errorf("telegram sendMessage: %s", resp.Description)
	}
	return resp.Result.MessageID, nil
}

type tgUpdate struct {
	UpdateID      int64 `json:"update_id"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
	} `json:"callback_query"`
}

// pollForCallback polls getUpdates every 2s until a callback matching uuid is found.
func (t *TelegramApprover) pollForCallback(ctx context.Context, uuid string) (bool, error) {
	var offset int64

	for {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("approval timed out: %w", ctx.Err())
		default:
		}

		updates, nextOffset, err := t.getUpdates(ctx, offset)
		if err != nil {
			// On network error, wait and retry.
			select {
			case <-ctx.Done():
				return false, fmt.Errorf("approval timed out: %w", ctx.Err())
			case <-time.After(2 * time.Second):
				continue
			}
		}
		offset = nextOffset

		for _, u := range updates {
			if u.CallbackQuery == nil {
				continue
			}
			parts := strings.SplitN(u.CallbackQuery.Data, ":", 2)
			if len(parts) != 2 || parts[1] != uuid {
				continue
			}
			_ = t.answerCallbackQuery(u.CallbackQuery.ID)
			return parts[0] == "approve", nil
		}

		select {
		case <-ctx.Done():
			return false, fmt.Errorf("approval timed out: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// getUpdates polls Telegram for new updates using long polling (30s server-side timeout).
func (t *TelegramApprover) getUpdates(ctx context.Context, offset int64) ([]tgUpdate, int64, error) {
	apiURL := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=30", telegramAPIBase, t.botToken, offset)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, offset, fmt.Errorf("build getUpdates request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, offset, fmt.Errorf("getUpdates: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, offset, fmt.Errorf("read getUpdates response: %w", err)
	}

	var result struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, offset, fmt.Errorf("decode getUpdates: %w", err)
	}

	nextOffset := offset
	for _, u := range result.Result {
		if u.UpdateID+1 > nextOffset {
			nextOffset = u.UpdateID + 1
		}
	}
	return result.Result, nextOffset, nil
}

// answerCallbackQuery dismisses the loading spinner on the inline button.
func (t *TelegramApprover) answerCallbackQuery(callbackQueryID string) error {
	payload := map[string]any{"callback_query_id": callbackQueryID}
	var resp struct{ OK bool `json:"ok"` }
	return t.post("answerCallbackQuery", payload, &resp)
}

// editMessage updates the text of a previously sent message.
func (t *TelegramApprover) editMessage(messageID int64, text string) error {
	payload := map[string]any{
		"chat_id":    t.chatID,
		"message_id": messageID,
		"text":       text,
	}
	var resp struct{ OK bool `json:"ok"` }
	return t.post("editMessageText", payload, &resp)
}

// post marshals payload as JSON and POSTs it to the given Telegram API method.
func (t *TelegramApprover) post(method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", method, err)
	}

	apiURL := fmt.Sprintf("%s%s/%s", telegramAPIBase, t.botToken, method)
	resp, err := t.client.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	return nil
}
