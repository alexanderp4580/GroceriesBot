package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

type Usage struct {
	Prompt     int
	Completion int
	Reasoning  int
	Cost       float64
	LatencyMS  int64
}

func (s *Store) Turn(ctx context.Context, id int64, round int) (json.RawMessage, error) {
	var b string
	e := s.db.QueryRowContext(ctx, "SELECT response_json FROM llm_turns WHERE update_id=? AND round_index=? AND state='completed'", id, round).Scan(&b)
	return json.RawMessage(b), e
}
func (s *Store) SaveTurn(ctx context.Context, id int64, round int, response json.RawMessage) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO llm_turns(update_id,round_index,request_json,response_json,state,created_at) VALUES(?,?,'{}',?,'completed',?) ON CONFLICT(update_id,round_index) DO NOTHING", id, round, string(response), stamp(time.Now()))
	return e
}
func (s *Store) Reserve(ctx context.Context, key string) error {
	cfg, e := s.Settings(ctx)
	if e != nil {
		return e
	}
	if !cfg.Bool("llm.enabled") {
		return errors.New("AI is disabled; no OpenRouter requests are permitted")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	var day, month int64
	for _, p := range []struct {
		since time.Time
		to    *int64
	}{{time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), &day}, {time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), &month}} {
		if e = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(cost_micros+reserved_micros),0) FROM llm_usage WHERE created_at>=?", stamp(p.since)).Scan(p.to); e != nil {
			return e
		}
	}
	const reservation = 50000
	if float64(day+reservation) > cfg.Float("llm.daily_budget_usd")*1e6 || float64(month+reservation) > cfg.Float("llm.monthly_budget_usd")*1e6 {
		return errors.New("AI spend limit reached")
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO llm_usage(request_key,created_at,state,reserved_micros) VALUES(?,?,'started',?)", key, stamp(now), reservation)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) RecordUsage(ctx context.Context, key string, u Usage) error {
	_, e := s.db.ExecContext(ctx, "UPDATE llm_usage SET state='completed',prompt_tokens=?,completion_tokens=?,reasoning_tokens=?,cost_micros=?,reserved_micros=0,latency_ms=? WHERE request_key=?", u.Prompt, u.Completion, u.Reasoning, int64(math.Round(u.Cost*1e6)), u.LatencyMS, key)
	return e
}
func (s *Store) UsageSummary(ctx context.Context) (string, error) {
	var n, p, c, r, spent, reserved, lat int64
	e := s.db.QueryRowContext(ctx, "SELECT count(*),COALESCE(SUM(prompt_tokens),0),COALESCE(SUM(completion_tokens),0),COALESCE(SUM(reasoning_tokens),0),COALESCE(SUM(cost_micros),0),COALESCE(SUM(reserved_micros),0),CAST(COALESCE(AVG(latency_ms),0) AS INTEGER) FROM llm_usage WHERE created_at>=?", stamp(time.Now().UTC().AddDate(0, 0, -30))).Scan(&n, &p, &c, &r, &spent, &reserved, &lat)
	return fmt.Sprintf("Last 30 days: %d model requests\nInput: %d tokens\nOutput: %d tokens (%d reasoning)\nRecorded cost: $%.4f\nUnresolved reservations: $%.4f\nAverage request: %d ms", n, p, c, r, float64(spent)/1e6, float64(reserved)/1e6, lat), e
}
func (s *Store) ActionCount(ctx context.Context, id int64) (int, error) {
	var n int
	e := s.db.QueryRowContext(ctx, "SELECT count(*) FROM mutation_journal WHERE update_id=?", id).Scan(&n)
	return n, e
}
func IsEmpty(e error) bool { return errors.Is(e, sql.ErrNoRows) }
