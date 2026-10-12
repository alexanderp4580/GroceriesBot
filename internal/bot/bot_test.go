package bot

import (
	"context"
	"encoding/json"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"github.com/alexanderp4580/GroceriesBot/internal/openrouter"
	"github.com/alexanderp4580/GroceriesBot/internal/sqlite"
	"path/filepath"
	"testing"
	"time"
)

type fixtureModel struct {
	Calls     int
	Responses []openrouter.Response
}

func (f *fixtureModel) Complete(context.Context, openrouter.Request) (openrouter.Response, error) {
	f.Calls++
	if len(f.Responses) == 0 {
		return openrouter.Response{Message: openrouter.Message{Role: "assistant", Content: `{"ordered_ids":[2,1]}`}}, nil
	}
	r := f.Responses[0]
	f.Responses = f.Responses[1:]
	return r, nil
}

type fixtureTelegram struct{}

func (f fixtureTelegram) Deliver(context.Context, grocery.Outgoing) (int64, error)   { return 10, nil }
func (f fixtureTelegram) Poll(context.Context, int64, int) ([]grocery.Update, error) { return nil, nil }
func (f fixtureTelegram) Download(context.Context, string, int64) ([]byte, string, error) {
	return []byte("fixture"), "image/png", nil
}
func fixture(t *testing.T) (*Bot, *sqlite.Store, *fixtureModel) {
	t.Helper()
	s, e := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	s.Bootstrap(context.Background(), []int64{-123})
	m := &fixtureModel{}
	return New(s, fixtureTelegram{}, m, "fixture-key"), s, m
}
func send(t *testing.T, b *Bot, s *sqlite.Store, u grocery.Update) {
	t.Helper()
	if e := s.Accept(context.Background(), []grocery.Update{u}, u.ID+1); e != nil {
		t.Fatal(e)
	}
	if e := b.Process(context.Background(), u); e != nil {
		t.Fatal(e)
	}
}
func TestCreditsGateAndCommandReplay(t *testing.T) {
	b, s, m := fixture(t)
	u := grocery.Update{ID: 1, ChatID: -123, UserID: 8, Text: "/add 2 carton Oat milk"}
	send(t, b, s, u)
	send(t, b, s, u)
	items, _ := s.Pending(context.Background(), -123)
	if len(items) != 1 || items[0].Unit != "carton" {
		t.Fatal(items)
	}
	send(t, b, s, grocery.Update{ID: 2, ChatID: -123, UserID: 8, Text: "milk"})
	send(t, b, s, grocery.Update{ID: 3, ChatID: -123, UserID: 8, Text: "/list"})
	if m.Calls != 0 {
		t.Fatal("spent model credits while disabled")
	}
}
func TestSignedPurchaseAndUserBoundConfirmation(t *testing.T) {
	b, s, _ := fixture(t)
	send(t, b, s, grocery.Update{ID: 1, ChatID: -123, UserID: 8, Text: "/add bread"})
	items, _ := s.Pending(context.Background(), -123)
	signed := b.callback("buy", items[0].ID, -123, 0)
	send(t, b, s, grocery.Update{ID: 2, ChatID: -123, UserID: 9, CallbackID: "cb", CallbackData: signed, MessageID: 20})
	pending, _ := s.Pending(context.Background(), -123)
	if len(pending) != 0 {
		t.Fatal("button failed")
	}
	send(t, b, s, grocery.Update{ID: 3, ChatID: -123, UserID: 8, Text: "/clear"})
	p, e := s.PendingAction(context.Background(), -123, 8)
	if e != nil {
		t.Fatal(e)
	}
	send(t, b, s, grocery.Update{ID: 4, ChatID: -123, UserID: 9, CallbackID: "other", CallbackData: b.callback("confirm", p.ID, -123, 8), MessageID: 21})
	history, _ := s.History(context.Background(), -123)
	if len(history) == 0 {
		t.Fatal("another user cleared list")
	}
}
func TestLLMOrderingHonoredAndCached(t *testing.T) {
	b, s, m := fixture(t)
	s.SetSetting(context.Background(), "llm.enabled", json.RawMessage(`true`))
	for n, name := range []string{"Bread", "Oat milk"} {
		send(t, b, s, grocery.Update{ID: int64(n + 1), ChatID: -123, UserID: 8, Text: "/add " + name})
	}
	u := grocery.Update{ID: 3, ChatID: -123, UserID: 8}
	s.Accept(context.Background(), []grocery.Update{u}, 4)
	out, e := b.list(context.Background(), u, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Buttons) < 2 || out.Buttons[0][0].Text != "✓ 1 × Oat milk" {
		t.Fatal(out)
	}
	_, e = b.list(context.Background(), u, 0, 0)
	if e != nil || m.Calls != 1 {
		t.Fatal("cache", m.Calls, e)
	}
}
func TestUnauthorizedDoesNotMutateOrCallModel(t *testing.T) {
	b, s, m := fixture(t)
	send(t, b, s, grocery.Update{ID: 1, ChatID: -999, UserID: 8, Text: "/add stolen"})
	items, _ := s.Pending(context.Background(), -999)
	if len(items) != 0 || m.Calls != 0 {
		t.Fatal("unauthorized side effect")
	}
}
func TestRestartReplaysAcceptedModelTurn(t *testing.T) {
	b, s, m := fixture(t)
	s.SetSetting(context.Background(), "llm.enabled", json.RawMessage(`true`))
	u := grocery.Update{ID: 7, ChatID: -123, UserID: 8, Text: "add oat milk"}
	s.Accept(context.Background(), []grocery.Update{u}, 8)
	r := openrouter.Response{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: "a", Type: "function", Function: openrouter.Function{Name: "add_items", Arguments: `{"items":[{"name":"Oat milk","category":"Dairy","quantity":1}]}`}}}}}
	raw, _ := json.Marshal(r)
	s.SaveTurn(context.Background(), 7, 0, raw)
	s.Apply(context.Background(), grocery.ActionKey{UpdateID: 7}, grocery.Operation{Kind: "add", ChatID: -123, UserID: 8, Items: []grocery.Item{{Name: "Oat milk", Category: "Dairy", Quantity: 1}}}, time.Now())
	m.Responses = []openrouter.Response{{Message: openrouter.Message{Role: "assistant", Content: "Added."}}}
	if e := b.Process(context.Background(), u); e != nil {
		t.Fatal(e)
	}
	items, _ := s.Pending(context.Background(), -123)
	if len(items) != 1 || m.Calls != 1 {
		t.Fatal("replayed mutation", items, m.Calls)
	}
}
func TestPhotoAndBatchNeedConfirmation(t *testing.T) {
	b, s, m := fixture(t)
	ctx := context.Background()
	s.SetSetting(ctx, "llm.enabled", json.RawMessage(`true`))
	m.Responses = []openrouter.Response{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: "a", Function: openrouter.Function{Name: "add_items", Arguments: `{"items":[{"name":"Bread"}]} `}}, {ID: "b", Function: openrouter.Function{Name: "add_items", Arguments: `{"items":[{"name":"Milk"}]}`}}}}}}
	send(t, b, s, grocery.Update{ID: 1, ChatID: -123, UserID: 8, Text: "add bread and milk"})
	xs, _ := s.Pending(ctx, -123)
	if len(xs) != 0 {
		t.Fatal("batch inserted without confirmation")
	}
	p, e := s.PendingAction(ctx, -123, 8)
	var values []grocery.Item
	json.Unmarshal(p.Payload, &values)
	if e != nil || len(values) != 2 {
		t.Fatal(p, e)
	}
}
func TestPriceOnlyCannotMutate(t *testing.T) {
	b, s, m := fixture(t)
	s.SetSetting(context.Background(), "llm.enabled", json.RawMessage(`true`))
	m.Responses = []openrouter.Response{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: "a", Function: openrouter.Function{Name: "add_items", Arguments: `{"items":[{"name":"Milk"}]}`}}}}}, {Message: openrouter.Message{Role: "assistant", Content: "An estimate."}}}
	send(t, b, s, grocery.Update{ID: 1, ChatID: -123, UserID: 8, Text: "How much would milk cost?"})
	xs, _ := s.Pending(context.Background(), -123)
	if len(xs) != 0 {
		t.Fatal("price query mutated list")
	}
}
func TestMalformedArgumentsAndZeroQuantity(t *testing.T) {
	b, s, _ := fixture(t)
	ctx := context.Background()
	u := grocery.Update{ID: 1, ChatID: -123, UserID: 8}
	s.Accept(ctx, []grocery.Update{u}, 2)
	for _, raw := range []string{`{"items":[{"name":"Milk","quantity":0}]}`, `{"items":[{"name":"Milk","added_at":"2020"}]}`, `{"items":[{"name":"Milk"}]} {}`} {
		_, _, _, e := b.execute(ctx, u, key(u), openrouter.ToolCall{Function: openrouter.Function{Name: "add_items", Arguments: raw}})
		if e == nil {
			t.Fatal(raw)
		}
	}
}
