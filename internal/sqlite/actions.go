package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"strings"
	"time"
)

func saved(ctx context.Context, tx *sql.Tx, k grocery.ActionKey, args string) (grocery.Result, bool, error) {
	var a, b string
	e := tx.QueryRowContext(ctx, "SELECT arguments_json,result_json FROM tool_actions WHERE update_id=? AND round_index=? AND action_index=?", k.UpdateID, k.RoundIndex, k.ActionIndex).Scan(&a, &b)
	if e == sql.ErrNoRows {
		return grocery.Result{}, false, nil
	}
	if e != nil {
		return grocery.Result{}, false, e
	}
	if a != args {
		return grocery.Result{}, false, errors.New("action replay arguments changed")
	}
	var r grocery.Result
	e = json.Unmarshal([]byte(b), &r)
	return r, true, e
}
func save(ctx context.Context, tx *sql.Tx, k grocery.ActionKey, name, args string, r grocery.Result, now time.Time) error {
	_, e := tx.ExecContext(ctx, "INSERT INTO tool_actions(update_id,round_index,action_index,tool_name,arguments_json,result_json,created_at) VALUES(?,?,?,?,?,?,?)", k.UpdateID, k.RoundIndex, k.ActionIndex, name, args, encoded(r), stamp(now))
	return e
}
func (s *Store) Apply(ctx context.Context, k grocery.ActionKey, op grocery.Operation, now time.Time) (grocery.Result, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return grocery.Result{}, e
	}
	defer tx.Rollback()
	args := encoded(op)
	if r, ok, e := saved(ctx, tx, k, args); e != nil || ok {
		return r, e
	}
	if op.ChatID == 0 || op.UserID <= 0 {
		return grocery.Result{}, errors.New("missing actor")
	}
	if op.Kind == "confirm_add" || op.Kind == "clear" {
		var p grocery.Pending
		p.ID = op.ConfirmationID
		var raw string
		e = tx.QueryRowContext(ctx, "SELECT chat_id,user_id,kind,payload_json,list_revision,expires_at FROM pending_actions WHERE id=?", p.ID).Scan(&p.ChatID, &p.UserID, &p.Kind, &raw, &p.Revision, &p.ExpiresAt)
		if e != nil || p.ChatID != op.ChatID || p.UserID != op.UserID || p.ExpiresAt <= stamp(now) {
			return grocery.Result{}, errors.New("confirmation is missing or expired")
		}
		var rev int64
		tx.QueryRowContext(ctx, "SELECT list_revision FROM chat_state WHERE chat_id=?", op.ChatID).Scan(&rev)
		if rev != p.Revision {
			return grocery.Result{}, errors.New("list changed; ask for confirmation again")
		}
		if op.Kind == "clear" && p.Kind != "confirm_clear" || op.Kind == "confirm_add" && p.Kind != "confirm_add" {
			return grocery.Result{}, errors.New("wrong confirmation kind")
		}
		if op.Kind == "confirm_add" {
			if e = json.Unmarshal([]byte(raw), &op.Items); e != nil {
				return grocery.Result{}, e
			}
			op.Kind = "add"
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM pending_actions WHERE id=?", p.ID); e != nil {
			return grocery.Result{}, e
		}
	}
	var before, after []grocery.Item
	result := grocery.Result{}
	switch op.Kind {
	case "add":
		var count int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM groceries WHERE chat_id=? AND purchased_at IS NULL AND deleted_at IS NULL", op.ChatID).Scan(&count); e != nil {
			return result, e
		}
		if count+len(op.Items) > 1000 {
			return result, errors.New("pending list capacity is 1000; purchase or remove items first")
		}
		if len(op.Items) == 0 || len(op.Items) > 100 {
			return result, errors.New("add requires 1 to 100 items")
		}
		for _, i := range op.Items {
			if e = grocery.ValidateItem(&i); e != nil {
				return result, e
			}
			var preferred string
			if e = tx.QueryRowContext(ctx, "SELECT preferred_name FROM aliases WHERE chat_id=? AND alias=?", op.ChatID, grocery.Normalize(i.Name)).Scan(&preferred); e == nil {
				i.Name = preferred
			} else if e != sql.ErrNoRows {
				return result, e
			}
			res, e := tx.ExecContext(ctx, "INSERT INTO groceries(chat_id,name,category,quantity,unit,emoji,added_at) VALUES(?,?,?,?,?,?,?)", op.ChatID, i.Name, i.Category, i.Quantity, i.Unit, i.Emoji, stamp(now))
			if e != nil {
				return result, e
			}
			i.ID, _ = res.LastInsertId()
			i.AddedAt = stamp(now)
			i.PurchasedAt = nil
			i.DeletedAt = nil
			i.Version = 1
			after = append(after, i)
		}
		result.Text = "📝 Added"
	case "clear":
		result, e = clearRows(ctx, tx, k, op, now)
		if e != nil {
			return result, e
		}
	case "purchase", "remove", "update":
		condition := "deleted_at IS NULL"
		if op.Kind != "clear" {
			condition += " AND purchased_at IS NULL"
		}
		all, e := items(ctx, tx, op.ChatID, condition)
		if e != nil {
			return result, e
		}
		targets := map[int64]bool{}
		for _, id := range op.IDs {
			if id <= 0 || targets[id] {
				return result, errors.New("invalid or duplicate item ID")
			}
			targets[id] = true
		}
		if op.Kind == "update" {
			if len(op.Items) != 1 {
				return result, errors.New("update requires one item")
			}
			targets[op.Items[0].ID] = true
		}
		for _, i := range all {
			if op.All || op.Kind == "clear" || targets[i.ID] {
				before = append(before, i)
				delete(targets, i.ID)
			}
		}
		if len(targets) > 0 {
			return result, errors.New("item missing, purchased, or belongs to another chat")
		}
		if len(before) == 0 {
			return result, errors.New("no pending items selected")
		}
		for _, i := range before {
			switch op.Kind {
			case "purchase":
				v := stamp(now)
				i.PurchasedAt = &v
			case "remove", "clear":
				v := stamp(now)
				i.DeletedAt = &v
			case "update":
				p := op.Items[0]
				if p.Name != "" {
					i.Name = p.Name
				}
				if p.Category != "" {
					i.Category = p.Category
				}
				if p.Quantity != 0 {
					i.Quantity = p.Quantity
				}
				if p.Unit != "" {
					i.Unit = p.Unit
				}
				if p.Emoji != "" {
					i.Emoji = p.Emoji
				}
				if e = grocery.ValidateItem(&i); e != nil {
					return result, e
				}
			}
			i.Version++
			_, e = tx.ExecContext(ctx, "UPDATE groceries SET name=?,category=?,quantity=?,unit=?,emoji=?,purchased_at=?,deleted_at=?,version=? WHERE id=? AND chat_id=?", i.Name, i.Category, i.Quantity, i.Unit, i.Emoji, i.PurchasedAt, i.DeletedAt, i.Version, i.ID, op.ChatID)
			if e != nil {
				return result, e
			}
			after = append(after, i)
		}
		result.Text = map[string]string{"purchase": "✅ Purchased", "remove": "❌ Removed", "update": "✏️ Updated", "clear": "🧹 Cleared"}[op.Kind]
	case "undo":
		var journal int64
		var b, a string
		e = tx.QueryRowContext(ctx, "SELECT id,before_json,after_json FROM mutation_journal WHERE chat_id=? AND user_id=? AND undone_at IS NULL ORDER BY id DESC LIMIT 1", op.ChatID, op.UserID).Scan(&journal, &b, &a)
		if e != nil {
			return result, errors.New("nothing to undo")
		}
		if json.Unmarshal([]byte(b), &before) != nil || json.Unmarshal([]byte(a), &after) != nil {
			return result, errors.New("invalid journal")
		}
		if len(after) == 0 {
			if e = undoClear(ctx, tx, journal, op.ChatID); e != nil {
				return result, e
			}
		}
		old := map[int64]grocery.Item{}
		for _, i := range before {
			old[i.ID] = i
		}
		for _, i := range after {
			var version int64
			e = tx.QueryRowContext(ctx, "SELECT version FROM groceries WHERE id=? AND chat_id=?", i.ID, op.ChatID).Scan(&version)
			if e != nil || version != i.Version {
				return result, errors.New("cannot undo: a later edit changed the items")
			}
			v, ok := old[i.ID]
			if !ok {
				v = i
				d := stamp(now)
				v.DeletedAt = &d
			}
			v.Version = version + 1
			_, e = tx.ExecContext(ctx, "UPDATE groceries SET name=?,category=?,quantity=?,unit=?,emoji=?,purchased_at=?,deleted_at=?,version=? WHERE id=? AND chat_id=?", v.Name, v.Category, v.Quantity, v.Unit, v.Emoji, v.PurchasedAt, v.DeletedAt, v.Version, v.ID, op.ChatID)
			if e != nil {
				return result, e
			}
			result.Items = append(result.Items, v)
		}
		if _, e = tx.ExecContext(ctx, "UPDATE mutation_journal SET undone_at=? WHERE id=?", stamp(now), journal); e != nil {
			return result, e
		}
		result.Text = "↩️ Undid your last action"
	default:
		return result, errors.New("unknown grocery operation")
	}
	if op.Kind != "undo" && op.Kind != "clear" {
		_, e = tx.ExecContext(ctx, "INSERT INTO mutation_journal(chat_id,user_id,update_id,before_json,after_json,created_at) VALUES(?,?,?,?,?,?)", op.ChatID, op.UserID, k.UpdateID, encoded(before), encoded(after), stamp(now))
		if e != nil {
			return result, e
		}
		result.Items = after
		for _, i := range after {
			result.Text += fmt.Sprintf("\n#%d %s", i.ID, grocery.Label(i))
		}
	}
	var pendingCount int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM groceries WHERE chat_id=? AND purchased_at IS NULL AND deleted_at IS NULL", op.ChatID).Scan(&pendingCount); e != nil {
		return result, e
	}
	if pendingCount > 1000 {
		return result, errors.New("undo would exceed the pending list capacity")
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO chat_state(chat_id,list_revision,updated_at) VALUES(?,1,?) ON CONFLICT(chat_id) DO UPDATE SET list_revision=list_revision+1,updated_at=excluded.updated_at", op.ChatID, stamp(now)); e != nil {
		return result, e
	}
	if op.Kind == "purchase" || op.Kind == "remove" {
		_, e = tx.ExecContext(ctx, "UPDATE layout_cache SET revision=(SELECT list_revision FROM chat_state WHERE chat_id=?),ids_json=(SELECT json_group_array(value) FROM json_each(layout_cache.ids_json) WHERE EXISTS (SELECT 1 FROM groceries g WHERE g.chat_id=layout_cache.chat_id AND g.id=value AND g.purchased_at IS NULL AND g.deleted_at IS NULL)) WHERE chat_id=? AND revision=(SELECT list_revision-1 FROM chat_state WHERE chat_id=?)", op.ChatID, op.ChatID, op.ChatID)
	} else {
		_, e = tx.ExecContext(ctx, "DELETE FROM layout_cache WHERE chat_id=?", op.ChatID)
	}
	if e != nil {
		return result, e
	}
	if e = save(ctx, tx, k, op.Kind, args, result, now); e != nil {
		return result, e
	}
	if e = tx.Commit(); e != nil {
		return result, e
	}
	return result, nil
}
func (s *Store) Prepare(ctx context.Context, k grocery.ActionKey, chat, user int64, kind string, payload any, now time.Time) (grocery.Pending, error) {
	cfg, e := s.Settings(ctx)
	if e != nil {
		return grocery.Pending{}, e
	}
	expiry := time.Duration(cfg.Int("confirm.expiry_seconds")) * time.Second
	args := encoded(struct {
		Chat, User int64
		Kind       string
		Payload    any
	}{chat, user, kind, payload})
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return grocery.Pending{}, e
	}
	defer tx.Rollback()
	if r, ok, e := saved(ctx, tx, k, args); e != nil {
		return grocery.Pending{}, e
	} else if ok {
		var p grocery.Pending
		e = json.Unmarshal([]byte(r.Text), &p)
		return p, e
	}
	var revision int64
	e = tx.QueryRowContext(ctx, "SELECT list_revision FROM chat_state WHERE chat_id=?", chat).Scan(&revision)
	if e != nil && e != sql.ErrNoRows {
		return grocery.Pending{}, e
	}
	_, e = tx.ExecContext(ctx, "DELETE FROM pending_actions WHERE chat_id=? AND user_id=?", chat, user)
	if e != nil {
		return grocery.Pending{}, e
	}
	res, e := tx.ExecContext(ctx, "INSERT INTO pending_actions(chat_id,user_id,kind,payload_json,list_revision,expires_at) VALUES(?,?,?,?,?,?)", chat, user, kind, encoded(payload), revision, stamp(now.Add(expiry)))
	if e != nil {
		return grocery.Pending{}, e
	}
	id, _ := res.LastInsertId()
	p := grocery.Pending{ID: id, ChatID: chat, UserID: user, Kind: kind, Payload: json.RawMessage(encoded(payload)), Revision: revision, ExpiresAt: stamp(now.Add(expiry))}
	e = save(ctx, tx, k, "prepare", args, grocery.Result{Text: encoded(p)}, now)
	if e != nil {
		return p, e
	}
	e = tx.Commit()
	return p, e
}
func (s *Store) PendingAction(ctx context.Context, chat, user int64) (grocery.Pending, error) {
	var p grocery.Pending
	var b string
	e := s.db.QueryRowContext(ctx, "SELECT id,kind,payload_json,list_revision,expires_at FROM pending_actions WHERE chat_id=? AND user_id=?", chat, user).Scan(&p.ID, &p.Kind, &b, &p.Revision, &p.ExpiresAt)
	p.ChatID = chat
	p.UserID = user
	p.Payload = json.RawMessage(b)
	return p, e
}
func (s *Store) Cancel(ctx context.Context, chat, user int64) error {
	_, e := s.db.ExecContext(ctx, "DELETE FROM pending_actions WHERE chat_id=? AND user_id=?", chat, user)
	return e
}
func (s *Store) SaveTemplate(ctx context.Context, chat int64, name string, values []grocery.Item) error {
	if len(name) == 0 || len(name) > 80 {
		return errors.New("template name is invalid")
	}
	for n := range values {
		values[n].ID = 0
		values[n].AddedAt = ""
		values[n].PurchasedAt = nil
		values[n].DeletedAt = nil
		values[n].Version = 0
	}
	_, e := s.db.ExecContext(ctx, "INSERT INTO templates(chat_id,name,items_json) VALUES(?,?,?) ON CONFLICT(chat_id,name) DO UPDATE SET items_json=excluded.items_json", chat, grocery.Normalize(name), encoded(values))
	return e
}
func (s *Store) Template(ctx context.Context, chat int64, name string) ([]grocery.Item, error) {
	var b string
	e := s.db.QueryRowContext(ctx, "SELECT items_json FROM templates WHERE chat_id=? AND name=?", chat, grocery.Normalize(name)).Scan(&b)
	var v []grocery.Item
	if e == nil {
		e = json.Unmarshal([]byte(b), &v)
	}
	return v, e
}
func (s *Store) Templates(ctx context.Context, chat int64) ([]string, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT name FROM templates WHERE chat_id=? ORDER BY name", chat)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if e = rows.Scan(&n); e != nil {
			return nil, e
		}
		names = append(names, n)
	}
	return names, rows.Err()
}
func (s *Store) SetAlias(ctx context.Context, chat int64, a, name string) error {
	if len(a) > 100 || len(name) > 300 || strings.TrimSpace(a) == "" || strings.TrimSpace(name) == "" {
		return errors.New("alias is invalid")
	}
	_, e := s.db.ExecContext(ctx, "INSERT INTO aliases(chat_id,alias,preferred_name) VALUES(?,?,?) ON CONFLICT(chat_id,alias) DO UPDATE SET preferred_name=excluded.preferred_name", chat, grocery.Normalize(a), strings.TrimSpace(name))
	return e
}
func (s *Store) Aliases(ctx context.Context, chat int64) (map[string]string, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT alias,preferred_name FROM aliases WHERE chat_id=? LIMIT 100", chat)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := map[string]string{}
	for rows.Next() {
		var a, n string
		if e = rows.Scan(&a, &n); e != nil {
			return nil, e
		}
		v[a] = n
	}
	return v, rows.Err()
}
func (s *Store) Action(ctx context.Context, k grocery.ActionKey) (grocery.Operation, grocery.Result, error) {
	var args, raw string
	var op grocery.Operation
	var result grocery.Result
	e := s.db.QueryRowContext(ctx, "SELECT arguments_json,result_json FROM tool_actions WHERE update_id=? AND round_index=? AND action_index=?", k.UpdateID, k.RoundIndex, k.ActionIndex).Scan(&args, &raw)
	if e != nil {
		return op, result, e
	}
	if e = json.Unmarshal([]byte(args), &op); e != nil {
		return op, result, e
	}
	e = json.Unmarshal([]byte(raw), &result)
	return op, result, e
}
