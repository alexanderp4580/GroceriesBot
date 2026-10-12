package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"time"
)

// TakeClarification consumes a question and saves its context atomically for crash replay.
func (s *Store) TakeClarification(ctx context.Context, u grocery.Update, id int64, now time.Time) (grocery.Pending, error) {
	k := grocery.ActionKey{UpdateID: u.ID, RoundIndex: -1}
	args := encoded([]int64{u.ChatID, u.UserID})
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return grocery.Pending{}, e
	}
	defer tx.Rollback()
	if r, ok, e := saved(ctx, tx, k, args); e != nil {
		return grocery.Pending{}, e
	} else if ok {
		var p grocery.Pending
		if e = json.Unmarshal([]byte(r.Text), &p); e != nil {
			return p, e
		}
		if id != 0 && id != p.ID {
			return p, errors.New("wrong clarification")
		}
		return p, nil
	}
	var p grocery.Pending
	var raw string
	e = tx.QueryRowContext(ctx, "SELECT id,kind,payload_json,list_revision,expires_at FROM pending_actions WHERE chat_id=? AND user_id=? AND kind='clarify'", u.ChatID, u.UserID).Scan(&p.ID, &p.Kind, &raw, &p.Revision, &p.ExpiresAt)
	if e != nil {
		return p, e
	}
	if id != 0 && id != p.ID || p.ExpiresAt <= stamp(now) {
		return p, sql.ErrNoRows
	}
	p.ChatID = u.ChatID
	p.UserID = u.UserID
	p.Payload = json.RawMessage(raw)
	if _, e = tx.ExecContext(ctx, "DELETE FROM pending_actions WHERE id=?", p.ID); e != nil {
		return p, e
	}
	if e = save(ctx, tx, k, "clarification", args, grocery.Result{Text: encoded(p)}, now); e != nil {
		return p, e
	}
	return p, tx.Commit()
}
