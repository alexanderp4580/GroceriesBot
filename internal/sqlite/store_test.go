package sqlite

import (
	"context"
	"encoding/json"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, e := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func seedUpdate(t *testing.T, s *Store, id int64) {
	t.Helper()
	if e := s.Accept(context.Background(), []grocery.Update{{ID: id, ChatID: -123, UserID: 8, Text: "test"}}, id+1); e != nil {
		t.Fatal(e)
	}
}
func TestReplayAndUndoPreserveItem(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seedUpdate(t, s, 1)
	op := grocery.Operation{Kind: "add", ChatID: -123, UserID: 8, Items: []grocery.Item{{Name: "Oat milk", Category: "Dairy", Quantity: 2, Unit: "carton"}}}
	key := grocery.ActionKey{UpdateID: 1}
	r, e := s.Apply(ctx, key, op, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Apply(ctx, key, op, time.Now())
	if e != nil || len(again.Items) != 1 || again.Items[0].ID != r.Items[0].ID {
		t.Fatal("replay", again, e)
	}
	items, _ := s.Pending(ctx, -123)
	if len(items) != 1 {
		t.Fatal("duplicated")
	}
	seedUpdate(t, s, 2)
	_, e = s.Apply(ctx, grocery.ActionKey{UpdateID: 2}, grocery.Operation{Kind: "purchase", ChatID: -123, UserID: 8, IDs: []int64{items[0].ID}}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	seedUpdate(t, s, 3)
	_, e = s.Apply(ctx, grocery.ActionKey{UpdateID: 3}, grocery.Operation{Kind: "undo", ChatID: -123, UserID: 8}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	pending, _ := s.Pending(ctx, -123)
	if len(pending) != 1 || pending[0].AddedAt != items[0].AddedAt {
		t.Fatal("undo failed", pending)
	}
}
func TestSettingsAndNumericConstraints(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if e := s.SetSetting(ctx, "llm.enabled", json.RawMessage(`false`)); e != nil {
		t.Fatal(e)
	}
	if e := s.SetSetting(ctx, "llm.max_tool_rounds", json.RawMessage(`"lots"`)); e == nil {
		t.Fatal("invalid setting")
	}
	_, e := s.db.Exec(`INSERT INTO groceries(chat_id,name,category,quantity,added_at) VALUES(1,'invalid','Pantry','many','2026-01-01T00:00:00Z')`)
	if e == nil {
		t.Fatal("accepted text quantity")
	}
}
func TestAcceptanceAndOrderCache(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seedUpdate(t, s, 1)
	seedUpdate(t, s, 1)
	u, e := s.NextUpdate(ctx)
	if e != nil || u.ID != 1 {
		t.Fatal(u, e)
	}
	if e = s.SaveLayout(ctx, -123, 0, "route", []int64{8, 4}); e != nil {
		t.Fatal(e)
	}
	ids, ok, e := s.Layout(ctx, -123, 0, "route")
	if e != nil || !ok || ids[0] != 8 {
		t.Fatal(ids, ok, e)
	}
	_, ok, _ = s.Layout(ctx, -123, 1, "route")
	if ok {
		t.Fatal("stale layout")
	}
}
func TestFractionalLatencySummary(t *testing.T) {
	s := openTest(t)
	s.db.Exec("INSERT INTO llm_usage(request_key,created_at,state,latency_ms) VALUES('a',?,'completed',1),('b',?,'completed',2)", stamp(time.Now()), stamp(time.Now()))
	if _, e := s.UsageSummary(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestLargeHistoryIsBounded(t *testing.T) {
	s := openTest(t)
	for n := 0; n < 1002; n++ {
		s.db.Exec("INSERT INTO groceries(chat_id,name,category,quantity,added_at,purchased_at) VALUES(-123,'old','Other',1,?,?)", stamp(time.Now()), stamp(time.Now()))
	}
	v, e := s.History(context.Background(), -123)
	if e != nil || len(v) != 20 {
		t.Fatal(len(v), e)
	}
}
func TestConfirmationExpirySetting(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seedUpdate(t, s, 1)
	s.SetSetting(ctx, "confirm.expiry_seconds", json.RawMessage(`10`))
	now := time.Now().UTC().Truncate(time.Second)
	p, e := s.Prepare(ctx, grocery.ActionKey{UpdateID: 1}, -123, 8, "confirm_clear", nil, now)
	if e != nil || p.ExpiresAt != stamp(now.Add(10*time.Second)) {
		t.Fatal(p, e)
	}
}
func TestReopenMigrationAndSettings(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "db")
	s, e := Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	s.SetSetting(ctx, "llm.enabled", json.RawMessage(`false`))
	s.SetSetting(ctx, "list.category_order", json.RawMessage(`["custom"]`))
	s.Close()
	s, e = Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	v, e := s.Settings(ctx)
	if e != nil || v.Strings("list.category_order")[0] != "custom" {
		t.Fatal(v, e)
	}
	if e = s.Check(ctx); e != nil {
		t.Fatal(e)
	}
}
func TestPruneProtectsUndeliveredState(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seedUpdate(t, s, 1)
	seedUpdate(t, s, 2)
	old := stamp(time.Now().AddDate(0, 0, -40))
	for _, id := range []int64{1, 2} {
		s.SaveTurn(ctx, id, 0, json.RawMessage(`{}`))
		s.Apply(ctx, grocery.ActionKey{UpdateID: id}, grocery.Operation{Kind: "add", ChatID: -123, UserID: 8, Items: []grocery.Item{{Name: "Milk", Quantity: 1}}}, time.Now())
		s.Complete(ctx, grocery.Update{ID: id, ChatID: -123, UserID: 8, Text: "fixture"}, []grocery.Outgoing{{Method: "sendMessage", ChatID: -123, Text: "done"}}, "done")
		s.db.Exec("UPDATE telegram_updates SET completed_at=? WHERE update_id=?", old, id)
	}
	r, e := s.NextReply(ctx)
	if e != nil {
		t.Fatal(e)
	}
	s.Sent(ctx, r, 100)
	if e = s.Prune(ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	var n int
	s.db.QueryRow("SELECT count(*) FROM telegram_updates").Scan(&n)
	if n != 1 {
		t.Fatal("prune", n)
	}
	s.db.QueryRow("SELECT count(*) FROM llm_turns WHERE update_id=2").Scan(&n)
	if n != 1 {
		t.Fatal("lost replay")
	}
}
func TestUndoConflictAndImmutableDates(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seedUpdate(t, s, 1)
	r, e := s.Apply(ctx, grocery.ActionKey{UpdateID: 1}, grocery.Operation{Kind: "add", ChatID: -123, UserID: 8, Items: []grocery.Item{{Name: "Milk", Quantity: 1}}}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	seedUpdate(t, s, 2)
	r2, e := s.Apply(ctx, grocery.ActionKey{UpdateID: 2}, grocery.Operation{Kind: "update", ChatID: -123, UserID: 9, Items: []grocery.Item{{ID: r.Items[0].ID, Name: "Oat milk", Quantity: 2, AddedAt: "injected"}}}, time.Now())
	if e != nil || r2.Items[0].AddedAt != r.Items[0].AddedAt {
		t.Fatal(r2, e)
	}
	seedUpdate(t, s, 3)
	_, e = s.Apply(ctx, grocery.ActionKey{UpdateID: 3}, grocery.Operation{Kind: "undo", ChatID: -123, UserID: 8}, time.Now())
	if e == nil {
		t.Fatal("undo overwrote another user's edit")
	}
}
func TestPendingCapacity(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for n := 0; n < 1000; n++ {
		s.db.Exec("INSERT INTO groceries(chat_id,name,category,quantity,added_at) VALUES(-123,'fixture','Other',1,?)", stamp(time.Now()))
	}
	seedUpdate(t, s, 1)
	_, e := s.Apply(ctx, grocery.ActionKey{UpdateID: 1}, grocery.Operation{Kind: "add", ChatID: -123, UserID: 8, Items: []grocery.Item{{Name: "Extra", Quantity: 1}}}, time.Now())
	if e == nil {
		t.Fatal("pending capacity exceeded")
	}
}
func TestClearLargeHistoryAndUndo(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for n := 0; n < 1001; n++ {
		s.db.Exec("INSERT INTO groceries(chat_id,name,category,quantity,added_at,purchased_at) VALUES(-123,'fixture','Other',1,?,?)", stamp(time.Now()), stamp(time.Now()))
	}
	seedUpdate(t, s, 1)
	p, e := s.Prepare(ctx, grocery.ActionKey{UpdateID: 1}, -123, 8, "confirm_clear", nil, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	seedUpdate(t, s, 2)
	_, e = s.Apply(ctx, grocery.ActionKey{UpdateID: 2}, grocery.Operation{Kind: "clear", ChatID: -123, UserID: 8, ConfirmationID: p.ID}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var n int
	s.db.QueryRow("SELECT count(*) FROM groceries WHERE deleted_at IS NULL").Scan(&n)
	if n != 0 {
		t.Fatal(n)
	}
	seedUpdate(t, s, 3)
	_, e = s.Apply(ctx, grocery.ActionKey{UpdateID: 3}, grocery.Operation{Kind: "undo", ChatID: -123, UserID: 8}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	s.db.QueryRow("SELECT count(*) FROM groceries WHERE deleted_at IS NULL").Scan(&n)
	if n != 1001 {
		t.Fatal(n)
	}
}
func TestDelayedEditCannotBeOvertaken(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for _, id := range []int64{1, 2} {
		seedUpdate(t, s, id)
		s.Complete(ctx, grocery.Update{ID: id, ChatID: -123, UserID: 8}, []grocery.Outgoing{{Method: "editMessageText", ChatID: -123, MessageID: 77, Text: "revision"}}, "")
	}
	r, e := s.NextReply(ctx)
	if e != nil {
		t.Fatal(e)
	}
	s.RetryReply(ctx, r, time.Hour, false)
	if _, e = s.NextReply(ctx); !IsEmpty(e) {
		t.Fatal("new edit overtook delayed older edit", e)
	}
}
func TestTerminalDeliveryFailureIsPruned(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seedUpdate(t, s, 1)
	u := grocery.Update{ID: 1, ChatID: -123, UserID: 8}
	s.Complete(ctx, u, []grocery.Outgoing{{Method: "sendMessage", ChatID: -123, Text: "fixture"}}, "")
	r, e := s.NextReply(ctx)
	if e != nil {
		t.Fatal(e)
	}
	s.RetryReply(ctx, r, 0, true)
	s.db.Exec("UPDATE telegram_updates SET completed_at=? WHERE update_id=1", stamp(time.Now().AddDate(0, 0, -40)))
	if e = s.Prune(ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	var n int
	s.db.QueryRow("SELECT count(*) FROM telegram_updates").Scan(&n)
	if n != 0 {
		t.Fatal("terminal delivery retained indefinitely")
	}
}
func TestMissingCostKeepsBudgetReservation(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	s.SetSetting(ctx, "llm.enabled", json.RawMessage(`true`))
	if e := s.Reserve(ctx, "unknown"); e != nil {
		t.Fatal(e)
	}
	if e := s.RecordUsage(ctx, "unknown", Usage{Prompt: 1000, Completion: 500}); e != nil {
		t.Fatal(e)
	}
	var reserve int
	s.db.QueryRow("SELECT reserved_micros FROM llm_usage WHERE request_key='unknown'").Scan(&reserve)
	if reserve != 50000 {
		t.Fatal("unknown bill released conservative allowance")
	}
}
func TestItemEmojiIsStoredEditedAndRestoredByUndo(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seedUpdate(t, s, 1)
	_, e := s.Apply(ctx, grocery.ActionKey{UpdateID: 1}, grocery.Operation{Kind: "add", ChatID: -123, UserID: 8, Items: []grocery.Item{{Name: "Lemons", Category: "Fruits", Quantity: 2, Emoji: "🍋"}}}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	items, _ := s.Pending(ctx, -123)
	if len(items) != 1 || items[0].Emoji != "🍋" {
		t.Fatal("emoji not stored", items)
	}
	seedUpdate(t, s, 2)
	r, e := s.Apply(ctx, grocery.ActionKey{UpdateID: 2}, grocery.Operation{Kind: "update", ChatID: -123, UserID: 8, Items: []grocery.Item{{ID: items[0].ID, Emoji: "🍊"}}}, time.Now())
	if e != nil || len(r.Items) != 1 || r.Items[0].Emoji != "🍊" {
		t.Fatal("emoji not edited", r, e)
	}
	seedUpdate(t, s, 3)
	if _, e = s.Apply(ctx, grocery.ActionKey{UpdateID: 3}, grocery.Operation{Kind: "undo", ChatID: -123, UserID: 8}, time.Now()); e != nil {
		t.Fatal(e)
	}
	items, _ = s.Pending(ctx, -123)
	if len(items) != 1 || items[0].Emoji != "🍋" {
		t.Fatal("undo did not restore emoji", items)
	}
}
