package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"time"
)

func clearRows(ctx context.Context, tx *sql.Tx, k grocery.ActionKey, op grocery.Operation, now time.Time) (grocery.Result, error) {
	var n int
	if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM groceries WHERE chat_id=? AND deleted_at IS NULL", op.ChatID).Scan(&n); e != nil {
		return grocery.Result{}, e
	}
	if n == 0 {
		return grocery.Result{}, errors.New("list is already empty")
	}
	res, e := tx.ExecContext(ctx, "INSERT INTO mutation_journal(chat_id,user_id,update_id,before_json,after_json,created_at) VALUES(?,?,?,'[]','[]',?)", op.ChatID, op.UserID, k.UpdateID, stamp(now))
	if e != nil {
		return grocery.Result{}, e
	}
	jid, _ := res.LastInsertId()
	_, e = tx.ExecContext(ctx, "INSERT INTO mutation_journal_items(journal_id,item_id,before_json,after_version) SELECT ?,id,json_object('deleted_at',deleted_at),version+1 FROM groceries WHERE chat_id=? AND deleted_at IS NULL", jid, op.ChatID)
	if e != nil {
		return grocery.Result{}, e
	}
	_, e = tx.ExecContext(ctx, "UPDATE groceries SET deleted_at=?,version=version+1 WHERE chat_id=? AND deleted_at IS NULL", stamp(now), op.ChatID)
	return grocery.Result{Text: fmt.Sprintf("🧹 Cleared %d items", n)}, e
}
func undoClear(ctx context.Context, tx *sql.Tx, jid, chat int64) error {
	var bad int
	e := tx.QueryRowContext(ctx, "SELECT count(*) FROM mutation_journal_items j LEFT JOIN groceries g ON g.id=j.item_id AND g.chat_id=? WHERE j.journal_id=? AND (g.id IS NULL OR g.version!=j.after_version)", chat, jid).Scan(&bad)
	if e != nil {
		return e
	}
	if bad != 0 {
		return errors.New("cannot undo: a later edit changed the items")
	}
	_, e = tx.ExecContext(ctx, "UPDATE groceries SET deleted_at=(SELECT json_extract(before_json,'$.deleted_at') FROM mutation_journal_items j WHERE j.journal_id=? AND j.item_id=groceries.id),version=version+1 WHERE chat_id=? AND id IN (SELECT item_id FROM mutation_journal_items WHERE journal_id=?)", jid, chat, jid)
	return e
}
