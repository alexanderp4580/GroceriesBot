package bot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"github.com/alexanderp4580/GroceriesBot/internal/openrouter"
	"github.com/alexanderp4580/GroceriesBot/internal/sqlite"
	"github.com/alexanderp4580/GroceriesBot/internal/telegram"
	"strconv"
	"strings"
	"time"
)

type Messenger interface {
	Poll(context.Context, int64, int) ([]grocery.Update, error)
	Deliver(context.Context, grocery.Outgoing) (int64, error)
	Download(context.Context, string, int64) ([]byte, string, error)
}
type Model interface {
	Complete(context.Context, openrouter.Request) (openrouter.Response, error)
}
type Bot struct {
	db       *sqlite.Store
	telegram Messenger
	model    Model
	secret   []byte
}

func New(db *sqlite.Store, t Messenger, m Model, secret string) *Bot {
	return &Bot{db, t, m, []byte(secret)}
}
func (b *Bot) callback(kind string, id, chat, user int64) string {
	body := fmt.Sprintf("gb:%s:%d", kind, id)
	h := hmac.New(sha256.New, b.secret)
	fmt.Fprintf(h, "%s:%d:%d", body, chat, user)
	return body + ":" + base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:10])
}
func (b *Bot) decode(u grocery.Update) (string, int64, error) {
	p := strings.Split(u.CallbackData, ":")
	if len(p) != 4 || p[0] != "gb" {
		return "", 0, errors.New("invalid button")
	}
	id, e := strconv.ParseInt(p[2], 10, 64)
	if e != nil {
		return "", 0, errors.New("invalid button")
	}
	user := int64(0)
	if p[1] == "confirm" || p[1] == "cancel" || strings.HasPrefix(p[1], "choice") {
		user = u.UserID
	}
	want := b.callback(p[1], id, u.ChatID, user)
	if !hmac.Equal([]byte(want), []byte(u.CallbackData)) {
		return "", 0, errors.New("this button belongs to another user or chat")
	}
	return p[1], id, nil
}
func text(u grocery.Update, s string) grocery.Outgoing {
	return grocery.Outgoing{Method: "sendMessage", ChatID: u.ChatID, Text: s}
}
func key(u grocery.Update) grocery.ActionKey { return grocery.ActionKey{UpdateID: u.ID} }
func (b *Bot) Process(ctx context.Context, u grocery.Update) error {
	done, e := b.db.Completed(ctx, u.ID)
	if e != nil || done {
		return e
	}
	cfg, e := b.db.Settings(ctx)
	if e != nil {
		return e
	}
	if u.UserID <= 0 || !cfg.Allowed(u.ChatID, u.UserID) {
		return b.db.Complete(ctx, u, nil, "")
	}
	if e = b.db.Claim(ctx, u.ID); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Int("processing.timeout_seconds"))*time.Second)
	defer cancel()
	var out []grocery.Outgoing
	if u.CallbackID != "" {
		out, e = b.handleCallback(ctx, u)
	} else if strings.HasPrefix(strings.TrimSpace(u.Text), "/") {
		out, e = b.command(ctx, u)
	} else if p, pe := b.db.PendingAction(ctx, u.ChatID, u.UserID); pe == nil && (p.Kind == "confirm_add" || p.Kind == "confirm_clear") && (strings.EqualFold(strings.TrimSpace(u.Text), "yes") || strings.EqualFold(strings.TrimSpace(u.Text), "confirm")) {
		out, e = b.confirm(ctx, u, p.ID)
	} else if !cfg.Bool("llm.enabled") {
		out = []grocery.Outgoing{text(u, "AI is paused to avoid OpenRouter charges. Use /add, /list, /done, /remove, /undo or /help.")}
	} else {
		out, e = b.converse(ctx, u, cfg)
	}
	// Completion gets its own short context so a processing deadline still produces a durable reply.
	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if e != nil {
		n, _ := b.db.ActionCount(finish, u.ID)
		msg := "Could not finish: " + e.Error()
		if n > 0 {
			msg += fmt.Sprintf("\n%d action(s) were saved. Use /list to check or /undo.", n)
		}
		out = []grocery.Outgoing{text(u, msg)}
	}
	if u.CallbackID != "" {
		out = append([]grocery.Outgoing{{Method: "answerCallbackQuery", ChatID: u.ChatID, CallbackID: u.CallbackID, Text: ""}}, out...)
	}
	memory := ""
	expanded := []grocery.Outgoing{}
	for _, o := range out {
		if o.Method == "sendMessage" {
			parts := telegram.SplitText(o.Text, 3800)
			for n, part := range parts {
				p := o
				p.Text = part
				if n < len(parts)-1 {
					p.Buttons = nil
				}
				expanded = append(expanded, p)
			}
		} else {
			expanded = append(expanded, o)
		}
		if o.Method != "answerCallbackQuery" {
			memory += o.Text + "\n"
		}
	}
	return b.db.Complete(finish, u, expanded, memory)
}
func (b *Bot) handleCallback(ctx context.Context, u grocery.Update) ([]grocery.Outgoing, error) {
	kind, id, e := b.decode(u)
	if e != nil {
		return nil, e
	}
	switch kind {
	case "buy":
		_, e = b.db.Apply(ctx, key(u), grocery.Operation{Kind: "purchase", ChatID: u.ChatID, UserID: u.UserID, IDs: []int64{id}}, time.Now())
		if e != nil {
			return nil, e
		}
		o, e := b.list(ctx, u, 0, u.MessageID)
		return []grocery.Outgoing{o}, e
	case "page":
		o, e := b.list(ctx, u, int(id), u.MessageID)
		return []grocery.Outgoing{o}, e
	case "confirm":
		return b.confirm(ctx, u, id)
	case "cancel":
		p, e := b.db.PendingAction(ctx, u.ChatID, u.UserID)
		if e != nil || p.ID != id {
			return nil, errors.New("confirmation expired")
		}
		e = b.db.Cancel(ctx, u.ChatID, u.UserID)
		return []grocery.Outgoing{text(u, "Cancelled.")}, e
	default:
		if strings.HasPrefix(kind, "choice") {
			n, e := strconv.Atoi(strings.TrimPrefix(kind, "choice"))
			if e != nil {
				return nil, errors.New("invalid choice")
			}
			p, e := b.db.PendingAction(ctx, u.ChatID, u.UserID)
			if e != nil || p.ID != id || p.ExpiresAt <= time.Now().UTC().Format(time.RFC3339) {
				return nil, errors.New("question expired")
			}
			var a clarification
			if decode(p.Payload, &a) != nil || n < 0 || n >= len(a.Options) {
				return nil, errors.New("invalid choice")
			}
			u.Text = a.Options[n]
			cfg, e := b.db.Settings(ctx)
			if e != nil {
				return nil, e
			}
			if !cfg.Bool("llm.enabled") {
				return nil, errors.New("AI is paused")
			}
			return b.converse(ctx, u, cfg)
		}
	}
	return nil, errors.New("unknown button")
}
func (b *Bot) confirm(ctx context.Context, u grocery.Update, id int64) ([]grocery.Outgoing, error) {
	p, e := b.db.PendingAction(ctx, u.ChatID, u.UserID)
	if e != nil || p.ID != id {
		return nil, errors.New("confirmation expired")
	}
	kind := "confirm_add"
	if p.Kind == "confirm_clear" {
		kind = "clear"
	}
	r, e := b.db.Apply(ctx, key(u), grocery.Operation{Kind: kind, ChatID: u.ChatID, UserID: u.UserID, ConfirmationID: id}, time.Now())
	if e != nil {
		return nil, e
	}
	return b.afterMutation(ctx, u, r.Text)
}
func (b *Bot) confirmation(u grocery.Update, p grocery.Pending, description string) grocery.Outgoing {
	o := text(u, description+"\nConfirm within five minutes.")
	o.Buttons = [][]grocery.Button{{{Text: "Confirm", Data: b.callback("confirm", p.ID, u.ChatID, u.UserID)}, {Text: "Cancel", Data: b.callback("cancel", p.ID, u.ChatID, u.UserID)}}}
	return o
}
func (b *Bot) afterMutation(ctx context.Context, u grocery.Update, summary string) ([]grocery.Outgoing, error) {
	out := []grocery.Outgoing{text(u, summary)}
	id, e := b.db.ShoppingMessage(ctx, u.ChatID)
	if e != nil {
		return nil, e
	}
	if id != 0 {
		o, e := b.list(ctx, u, 0, id)
		if e != nil {
			return out, nil
		}
		out = append(out, o)
	}
	return out, nil
}
