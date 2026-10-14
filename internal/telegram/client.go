package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf16"
)

type Client struct {
	token   string
	HTTP    *http.Client
	BaseURL string
	FileURL string
}
type APIError struct {
	Code       int
	RetryAfter int
	Permanent  bool
}

func (e *APIError) Error() string { return fmt.Sprintf("Telegram returned status %d", e.Code) }
func New(token string, h *http.Client) *Client {
	return &Client{token: token, HTTP: h, BaseURL: "https://api.telegram.org", FileURL: "https://api.telegram.org"}
}
func (c *Client) call(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	b, e := json.Marshal(payload)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/bot"+c.token+"/"+method, bytes.NewReader(b))
	if e != nil {
		return nil, errors.New("invalid Telegram request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, errors.New("Telegram connection failed")
	}
	defer resp.Body.Close()
	b, e = io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if e != nil || len(b) > 1024*1024 {
		return nil, errors.New("Telegram response exceeds limit")
	}
	var out struct {
		OK         bool            `json:"ok"`
		Result     json.RawMessage `json:"result"`
		Code       int             `json:"error_code"`
		Parameters struct {
			Retry int `json:"retry_after"`
		} `json:"parameters"`
	}
	if e = json.Unmarshal(b, &out); e != nil {
		return nil, errors.New("invalid Telegram response")
	}
	if !out.OK {
		return nil, &APIError{Code: out.Code, RetryAfter: out.Parameters.Retry, Permanent: out.Code >= 400 && out.Code < 500 && out.Code != 429}
	}
	return out.Result, nil
}

type actor struct {
	ID  int64 `json:"id"`
	Bot bool  `json:"is_bot"`
}
type message struct {
	ID   int64 `json:"message_id"`
	From actor `json:"from"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text    string   `json:"text"`
	Caption string   `json:"caption"`
	Reply   *message `json:"reply_to_message"`
	Photos  []struct {
		ID   string `json:"file_id"`
		Size int64  `json:"file_size"`
	} `json:"photo"`
}

func (c *Client) Poll(ctx context.Context, offset int64, timeout int) ([]grocery.Update, error) {
	raw, e := c.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": timeout, "limit": 50, "allowed_updates": []string{"message", "callback_query"}})
	if e != nil {
		return nil, e
	}
	var us []struct {
		ID       int64    `json:"update_id"`
		Message  *message `json:"message"`
		Callback *struct {
			ID      string  `json:"id"`
			From    actor   `json:"from"`
			Data    string  `json:"data"`
			Message message `json:"message"`
		} `json:"callback_query"`
	}
	if json.Unmarshal(raw, &us) != nil {
		return nil, errors.New("invalid updates")
	}
	out := []grocery.Update{}
	for _, v := range us {
		u := grocery.Update{ID: v.ID}
		if v.Message != nil {
			m := v.Message
			u.ChatID = m.Chat.ID
			u.UserID = m.From.ID
			u.MessageID = m.ID
			u.Text = m.Text
			if u.Text == "" {
				u.Text = m.Caption
			}
			if m.Reply != nil {
				u.ReplyTo = m.Reply.Text
			}
			if len(m.Photos) > 0 {
				for i := len(m.Photos) - 1; i >= 0; i-- {
					if m.Photos[i].Size <= 4194304 {
						u.PhotoID = m.Photos[i].ID
						break
					}
				}
				if u.PhotoID == "" {
					u.Text = "Photo exceeds the supported size."
				}
			}
			if m.From.Bot {
				u.UserID = 0
			}
		}
		if v.Callback != nil {
			m := v.Callback
			u.ChatID = m.Message.Chat.ID
			u.UserID = m.From.ID
			u.MessageID = m.Message.ID
			u.CallbackID = m.ID
			u.CallbackData = m.Data
		}
		out = append(out, u)
	}
	return out, nil
}
func (c *Client) Deliver(ctx context.Context, m grocery.Outgoing) (int64, error) {
	allowed := map[string]bool{"sendMessage": true, "editMessageText": true, "answerCallbackQuery": true}
	if !allowed[m.Method] {
		return 0, errors.New("unsupported Telegram method")
	}
	payload := map[string]any{}
	if m.Method == "answerCallbackQuery" {
		payload["callback_query_id"] = m.CallbackID
		payload["text"] = m.Text
	} else {
		payload["chat_id"] = m.ChatID
		payload["text"] = m.Text
		if m.Method == "editMessageText" {
			payload["message_id"] = m.MessageID
		}
		if m.Buttons != nil {
			payload["reply_markup"] = map[string]any{"inline_keyboard": m.Buttons}
		}
	}
	raw, e := c.call(ctx, m.Method, payload)
	if e != nil {
		return 0, e
	}
	var r struct {
		ID int64 `json:"message_id"`
	}
	json.Unmarshal(raw, &r)
	return r.ID, nil
}
func (c *Client) Download(ctx context.Context, fileID string, max int64) ([]byte, string, error) {
	raw, e := c.call(ctx, "getFile", map[string]string{"file_id": fileID})
	if e != nil {
		return nil, "", e
	}
	var f struct {
		Path string `json:"file_path"`
		Size int64  `json:"file_size"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Path == "" || strings.Contains(f.Path, "..") {
		return nil, "", errors.New("invalid photo metadata")
	}
	if f.Size > max {
		return nil, "", errors.New("photo exceeds size limit")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", c.FileURL+"/file/bot"+c.token+"/"+f.Path, nil)
	if e != nil {
		return nil, "", errors.New("invalid photo path")
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, "", errors.New("photo download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", errors.New("photo download rejected")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if e != nil || int64(len(b)) > max {
		return nil, "", errors.New("photo exceeds size limit")
	}
	kind := http.DetectContentType(b)
	if kind != "image/jpeg" && kind != "image/png" && kind != "image/webp" {
		return nil, "", errors.New("unsupported image type")
	}
	return b, kind, nil
}
func (c *Client) Webhook(ctx context.Context) (json.RawMessage, error) {
	return c.call(ctx, "getWebhookInfo", map[string]any{})
}
func (c *Client) RemoveWebhook(ctx context.Context) error {
	_, e := c.call(ctx, "deleteWebhook", map[string]bool{"drop_pending_updates": false})
	return e
}

// Command is one entry of the Telegram command menu; Command has no leading slash.
type Command struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

func (c *Client) SetCommands(ctx context.Context, commands []Command) error {
	_, e := c.call(ctx, "setMyCommands", map[string]any{"commands": commands})
	return e
}
func SplitText(s string, max int) []string {
	var out []string
	var b strings.Builder
	n := 0
	for _, r := range s {
		width := len(utf16.Encode([]rune{r}))
		if n+width > max {
			out = append(out, b.String())
			b.Reset()
			n = 0
		}
		b.WriteRune(r)
		n += width
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
func HTTPClient() *http.Client {
	return &http.Client{Timeout: 70 * time.Second, Transport: &http.Transport{MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 90 * time.Second}}
}
