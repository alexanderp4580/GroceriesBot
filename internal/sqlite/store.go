package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/config"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"github.com/alexanderp4580/GroceriesBot/migrations"
	_ "github.com/mattn/go-sqlite3"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }
func encoded(v any) string     { b, _ := json.Marshal(v); return string(b) }
func Open(ctx context.Context, path string) (*Store, error) {
	abs, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(abs), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	f.Close()
	if e = os.Chmod(abs, 0600); e != nil {
		return nil, e
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	for k, v := range map[string]string{"_foreign_keys": "on", "_journal_mode": "WAL", "_synchronous": "FULL", "_busy_timeout": "5000", "_cache_size": "-2048"} {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	db, e := sql.Open("sqlite3", u.String())
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	if e = s.migrate(ctx); e != nil {
		db.Close()
		return nil, e
	}
	if _, e = db.ExecContext(ctx, "PRAGMA mmap_size=0; PRAGMA wal_autocheckpoint=256;"); e != nil {
		db.Close()
		return nil, e
	}
	if e = s.seed(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Check(ctx context.Context) error {
	var v string
	if e := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&v); e != nil {
		return e
	}
	if v != "ok" {
		return errors.New("database integrity failure")
	}
	rows, e := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if e != nil {
		return e
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("database relationship failure")
	}
	return rows.Err()
}
func (s *Store) migrate(ctx context.Context) error {
	var version int
	if e := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	entries, e := migrations.Files.ReadDir(".")
	if e != nil {
		return e
	}
	var names []string
	for _, f := range entries {
		if strings.HasSuffix(f.Name(), ".sql") {
			names = append(names, f.Name())
		}
	}
	sort.Strings(names)
	if version > len(names) {
		return errors.New("database schema is newer than executable")
	}
	for i, name := range names {
		body, e := migrations.Files.ReadFile(name)
		if e != nil {
			return e
		}
		if i+1 <= version {
			continue
		}
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, string(body)); e == nil {
			_, e = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", i+1))
		}
		if e == nil && i+1 >= 2 {
			h := sha256.Sum256(body)
			_, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,applied_at,checksum) VALUES(?,?,?)", i+1, stamp(time.Now()), hex.EncodeToString(h[:]))
		}
		if e != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d failed: %w", i+1, e)
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	for i, name := range names {
		body, _ := migrations.Files.ReadFile(name)
		h := sha256.Sum256(body)
		want := hex.EncodeToString(h[:])
		var got string
		if e = s.db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=?", i+1).Scan(&got); e != nil {
			return e
		}
		if got != "" && got != want {
			return fmt.Errorf("migration %d checksum mismatch", i+1)
		}
		if got == "" {
			if _, e = s.db.ExecContext(ctx, "UPDATE schema_migrations SET checksum=? WHERE version=?", want, i+1); e != nil {
				return e
			}
		}
	}
	return nil
}
func Defaults() (config.Settings, error) {
	b, e := migrations.Files.ReadFile("defaults.json")
	if e != nil {
		return nil, e
	}
	var v config.Settings
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) seed(ctx context.Context) error {
	defaults, e := Defaults()
	if e != nil {
		return e
	}
	defaults["telegram.allowed_chat_ids"] = json.RawMessage(`[]`)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for k, v := range defaults {
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO settings(key,value_json,updated_at) VALUES(?,?,?)", k, string(v), stamp(time.Now())); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) Settings(ctx context.Context) (config.Settings, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT key,value_json FROM settings")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := config.Settings{}
	for rows.Next() {
		var k, b string
		if e = rows.Scan(&k, &b); e != nil {
			return nil, e
		}
		v[k] = json.RawMessage(b)
	}
	return v, rows.Err()
}
func (s *Store) SetSetting(ctx context.Context, k string, v json.RawMessage) error {
	defaults, e := Defaults()
	if e != nil {
		return e
	}
	if k == "telegram.allowed_chat_ids" {
		defaults[k] = json.RawMessage(`[]`)
	}
	d, ok := defaults[k]
	if !ok {
		return errors.New("unknown setting")
	}
	var a, b any
	if json.Unmarshal(d, &a) != nil || json.Unmarshal(v, &b) != nil {
		return errors.New("invalid setting JSON")
	}
	switch a.(type) {
	case bool:
		if _, ok = b.(bool); !ok {
			return errors.New("setting requires boolean")
		}
	case string:
		if _, ok = b.(string); !ok {
			return errors.New("setting requires text")
		}
	case float64:
		n, ok := b.(float64)
		if !ok || n < 0 || n > 1e6 {
			return errors.New("setting requires bounded number")
		}
		if !strings.Contains(k, "budget_usd") && n != float64(int(n)) {
			return errors.New("setting requires integer")
		}
		if !strings.Contains(k, "budget_usd") && n < 1 {
			return errors.New("setting must be positive")
		}
	case []any:
		xs, ok := b.([]any)
		if !ok {
			return errors.New("setting requires array")
		}
		for _, x := range xs {
			if strings.Contains(k, "_ids") {
				n, ok := x.(float64)
				if !ok || n == 0 || n != float64(int64(n)) {
					return errors.New("invalid identity")
				}
			} else if _, ok := x.(string); !ok {
				return errors.New("array requires strings")
			}
		}
	}
	if k == "llm.reasoning_effort" {
		x := b.(string)
		if x != "low" && x != "medium" && x != "high" {
			return errors.New("invalid effort")
		}
	}
	if k == "list.ordering_mode" && b != "llm" {
		return errors.New("ordering must be llm")
	}
	limits := map[string][2]int{"llm.max_tool_rounds": {1, 12}, "llm.max_output_tokens": {2048, 16384}, "memory.max_messages_per_chat": {2, 100}, "photos.max_bytes": {1024, 4194304}, "llm.context_max_chars": {1000, 24000}, "telegram.poll_timeout_seconds": {1, 50}}
	if r, ok := limits[k]; ok {
		n := int(b.(float64))
		if n < r[0] || n > r[1] {
			return errors.New("setting outside supported limits")
		}
	}
	_, e = s.db.ExecContext(ctx, "UPDATE settings SET value_json=?,updated_at=? WHERE key=?", string(v), stamp(time.Now()), k)
	return e
}
func (s *Store) Bootstrap(ctx context.Context, chats []int64) error {
	v, e := s.Settings(ctx)
	if e != nil {
		return e
	}
	if len(v.IDs("telegram.allowed_chat_ids")) == 0 && len(chats) > 0 {
		return s.SetSetting(ctx, "telegram.allowed_chat_ids", json.RawMessage(encoded(chats)))
	}
	return nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

const itemColumns = "id,name,category,quantity,unit,emoji,added_at,purchased_at,deleted_at,version"

func items(ctx context.Context, q queryer, chat int64, condition string, args ...any) ([]grocery.Item, error) {
	args = append([]any{chat}, args...)
	rows, e := q.QueryContext(ctx, "SELECT "+itemColumns+" FROM groceries WHERE chat_id=? AND "+condition+" ORDER BY id LIMIT 1001", args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []grocery.Item{}
	for rows.Next() {
		var i grocery.Item
		if e = rows.Scan(&i.ID, &i.Name, &i.Category, &i.Quantity, &i.Unit, &i.Emoji, &i.AddedAt, &i.PurchasedAt, &i.DeletedAt, &i.Version); e != nil {
			return nil, e
		}
		v = append(v, i)
	}
	if len(v) > 1000 {
		return nil, errors.New("list exceeds supported 1000 items")
	}
	return v, rows.Err()
}
func (s *Store) Pending(ctx context.Context, chat int64) ([]grocery.Item, error) {
	return items(ctx, s.db, chat, "purchased_at IS NULL AND deleted_at IS NULL")
}
func (s *Store) History(ctx context.Context, chat int64) ([]grocery.Item, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT "+itemColumns+" FROM groceries WHERE chat_id=? AND deleted_at IS NULL ORDER BY id DESC LIMIT 20", chat)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []grocery.Item{}
	for rows.Next() {
		var i grocery.Item
		if e = rows.Scan(&i.ID, &i.Name, &i.Category, &i.Quantity, &i.Unit, &i.Emoji, &i.AddedAt, &i.PurchasedAt, &i.DeletedAt, &i.Version); e != nil {
			return nil, e
		}
		v = append(v, i)
	}
	return v, rows.Err()
}

func (s *Store) Revision(ctx context.Context, chat int64) (int64, error) {
	var n int64
	e := s.db.QueryRowContext(ctx, "SELECT list_revision FROM chat_state WHERE chat_id=?", chat).Scan(&n)
	if e == sql.ErrNoRows {
		return 0, nil
	}
	return n, e
}
func (s *Store) Layout(ctx context.Context, chat, rev int64, fp string) ([]int64, bool, error) {
	var raw string
	e := s.db.QueryRowContext(ctx, "SELECT ids_json FROM layout_cache WHERE chat_id=? AND revision=? AND fingerprint=?", chat, rev, fp).Scan(&raw)
	if e == sql.ErrNoRows {
		return nil, false, nil
	}
	var ids []int64
	if e == nil {
		e = json.Unmarshal([]byte(raw), &ids)
	}
	return ids, e == nil, e
}
func (s *Store) SaveLayout(ctx context.Context, chat, rev int64, fp string, ids []int64) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO layout_cache(chat_id,revision,fingerprint,ids_json) VALUES(?,?,?,?) ON CONFLICT(chat_id) DO UPDATE SET revision=excluded.revision,fingerprint=excluded.fingerprint,ids_json=excluded.ids_json", chat, rev, fp, encoded(ids))
	return e
}
func (s *Store) Accept(ctx context.Context, us []grocery.Update, offset int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var seq int64
	if e = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(intake_seq),0) FROM telegram_updates").Scan(&seq); e != nil {
		return e
	}
	for _, u := range us {
		seq++
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO telegram_updates(update_id,chat_id,user_id,payload_json,received_at,intake_seq) VALUES(?,?,?,?,?,?)", u.ID, u.ChatID, u.UserID, encoded(u), stamp(time.Now()), seq); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO transport_state(key,value) VALUES('poll_offset',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.FormatInt(offset, 10)); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Offset(ctx context.Context) (int64, error) {
	var raw string
	e := s.db.QueryRowContext(ctx, "SELECT value FROM transport_state WHERE key='poll_offset'").Scan(&raw)
	if e == sql.ErrNoRows {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	return strconv.ParseInt(raw, 10, 64)
}
func (s *Store) Recover(ctx context.Context) error {
	_, e := s.db.ExecContext(ctx, "UPDATE telegram_updates SET state='pending' WHERE state='processing'")
	return e
}
func (s *Store) NextUpdate(ctx context.Context) (grocery.Update, error) {
	var b string
	e := s.db.QueryRowContext(ctx, "SELECT payload_json FROM telegram_updates WHERE state='pending' AND (available_at IS NULL OR available_at<=?) ORDER BY intake_seq LIMIT 1", stamp(time.Now())).Scan(&b)
	var u grocery.Update
	if e == nil {
		e = json.Unmarshal([]byte(b), &u)
	}
	return u, e
}
func (s *Store) Claim(ctx context.Context, id int64) error {
	_, e := s.db.ExecContext(ctx, "UPDATE telegram_updates SET state='processing',attempts=attempts+1 WHERE update_id=? AND state='pending'", id)
	return e
}
func (s *Store) Complete(ctx context.Context, u grocery.Update, out []grocery.Outgoing, memory string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var state string
	if e = tx.QueryRowContext(ctx, "SELECT state FROM telegram_updates WHERE update_id=?", u.ID).Scan(&state); e != nil {
		return e
	}
	if state == "completed" {
		return nil
	}
	for n, m := range out {
		if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO outbox(update_id,chat_id,sequence,payload_json,created_at) VALUES(?,?,?,?,?)", u.ID, u.ChatID, n, encoded(m), stamp(time.Now())); e != nil {
			return e
		}
	}
	if memory != "" {
		for _, msg := range []struct{ role, text string }{{"user", u.Text}, {"assistant", memory}} {
			if _, e = tx.ExecContext(ctx, "INSERT INTO conversation_messages(chat_id,user_id,role,content,update_id,created_at) VALUES(?,?,?,?,?,?)", u.ChatID, u.UserID, msg.role, msg.text, u.ID, stamp(time.Now())); e != nil {
				return e
			}
		}
	}
	_, e = tx.ExecContext(ctx, "UPDATE telegram_updates SET state='completed',completed_at=? WHERE update_id=?", stamp(time.Now()), u.ID)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Recent(ctx context.Context, chat int64, limit int) ([]map[string]string, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT role,content FROM (SELECT id,role,content FROM conversation_messages WHERE chat_id=? ORDER BY id DESC LIMIT ?) ORDER BY id", chat, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var role, text string
		if e = rows.Scan(&role, &text); e != nil {
			return nil, e
		}
		out = append(out, map[string]string{"role": role, "content": text})
	}
	return out, rows.Err()
}
func (s *Store) NextReply(ctx context.Context) (grocery.Reply, error) {
	var r grocery.Reply
	var b string
	e := s.db.QueryRowContext(ctx, "SELECT id,attempts,payload_json FROM outbox o WHERE state='pending' AND (available_at IS NULL OR available_at<=?) AND NOT EXISTS (SELECT 1 FROM outbox earlier WHERE earlier.chat_id=o.chat_id AND earlier.id<o.id AND earlier.state='pending') ORDER BY id LIMIT 1", stamp(time.Now())).Scan(&r.ID, &r.Attempts, &b)
	if e == nil {
		e = json.Unmarshal([]byte(b), &r.Message)
	}
	return r, e
}
func (s *Store) Sent(ctx context.Context, r grocery.Reply, messageID int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, "UPDATE outbox SET state='sent',sent_at=?,telegram_message_id=? WHERE id=?", stamp(time.Now()), messageID, r.ID)
	if e != nil {
		return e
	}
	if messageID != 0 && len(r.Message.Buttons) > 0 && r.Message.Method == "sendMessage" && strings.HasPrefix(r.Message.Text, "Shopping list") {
		_, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO shopping_messages(chat_id,message_id) VALUES(?,?)", r.Message.ChatID, messageID)
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) RetryReply(ctx context.Context, r grocery.Reply, delay time.Duration, permanent bool) error {
	state := "pending"
	if permanent || r.Attempts >= 5 {
		state = "failed"
	}
	_, e := s.db.ExecContext(ctx, "UPDATE outbox SET attempts=attempts+1,state=?,available_at=? WHERE id=?", state, stamp(time.Now().Add(delay)), r.ID)
	return e
}
func (s *Store) Prune(ctx context.Context, now time.Time) error {
	cfg, e := s.Settings(ctx)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, "DELETE FROM conversation_messages WHERE created_at<? OR id NOT IN (SELECT id FROM conversation_messages c2 WHERE c2.chat_id=conversation_messages.chat_id ORDER BY id DESC LIMIT ?)", stamp(now.AddDate(0, 0, -cfg.Int("memory.max_age_days"))), cfg.Int("memory.max_messages_per_chat"))
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "DELETE FROM pending_actions WHERE expires_at<?", stamp(now))
	if e != nil {
		return e
	}
	cut := stamp(now.AddDate(0, 0, -cfg.Int("updates.retention_days")))
	for _, table := range []string{"llm_turns", "tool_actions", "outbox"} {
		condition := ""
		if table == "outbox" {
			condition = " AND state IN ('sent','failed')"
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE update_id IN (SELECT update_id FROM telegram_updates WHERE state='completed' AND completed_at<? AND update_id NOT IN (SELECT update_id FROM outbox WHERE state NOT IN ('sent','failed')))"+condition, cut)
		if e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, "DELETE FROM telegram_updates WHERE state='completed' AND completed_at<? AND update_id NOT IN (SELECT update_id FROM outbox)", cut)
	if e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) Completed(ctx context.Context, id int64) (bool, error) {
	var state string
	e := s.db.QueryRowContext(ctx, "SELECT state FROM telegram_updates WHERE update_id=?", id).Scan(&state)
	return state == "completed", e
}
func (s *Store) ShoppingMessage(ctx context.Context, chat int64) (int64, error) {
	var id int64
	e := s.db.QueryRowContext(ctx, "SELECT message_id FROM shopping_messages WHERE chat_id=? ORDER BY message_id DESC LIMIT 1", chat).Scan(&id)
	if e == sql.ErrNoRows {
		return 0, nil
	}
	return id, e
}

func (s *Store) QueueCallbackAnswer(ctx context.Context, u grocery.Update) error {
	out := grocery.Outgoing{Method: "answerCallbackQuery", ChatID: u.ChatID, CallbackID: u.CallbackID}
	_, e := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO outbox(update_id,chat_id,sequence,payload_json,created_at) VALUES(?,?,-1,?,?)", u.ID, u.ChatID, encoded(out), stamp(time.Now()))
	return e
}
