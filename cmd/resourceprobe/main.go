// Resource probe: synthetic data, local fixture provider, real processor/SQLite/HTTP paths.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/bot"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"github.com/alexanderp4580/GroceriesBot/internal/openrouter"
	"github.com/alexanderp4580/GroceriesBot/internal/sqlite"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"
)

type transport struct {
	photo     []byte
	delivered atomic.Int64
}

func (t *transport) Poll(ctx context.Context, _ int64, _ int) ([]grocery.Update, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (t *transport) Deliver(_ context.Context, o grocery.Outgoing) (int64, error) {
	if o.Method == "answerCallbackQuery" {
		return 0, nil
	}
	return t.delivered.Add(1), nil
}
func (t *transport) Download(context.Context, string, int64) ([]byte, string, error) {
	p := make([]byte, 4194304)
	copy(p, t.photo)
	return p, "image/png", nil
}
func sign(kind string, id, user int64) string {
	body := fmt.Sprintf("gb:%s:%d", kind, id)
	h := hmac.New(sha256.New, []byte("fixture"))
	fmt.Fprintf(h, "%s:%d:%d", body, int64(-123), user)
	return body + ":" + base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:10])
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return errors.New("usage: resourceprobe PHOTO CAPTURED_PHOTO_RESPONSE")
	}
	photo, e := os.ReadFile(os.Args[1])
	if e != nil {
		return e
	}
	captured, e := os.ReadFile(os.Args[2])
	if e != nil {
		return e
	}
	var wire map[string]any
	if e = json.Unmarshal(captured, &wire); e != nil {
		return e
	}
	wire["usage"] = map[string]any{"cost": 0, "prompt_tokens": 0, "completion_tokens": 0}
	photoResponse, _ := json.Marshal(wire)
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"usage": 0, "usage_daily": 0, "usage_monthly": 0, "limit": nil, "limit_remaining": nil, "total_credits": 10, "total_usage": 0}})
			return
		}
		body, e := io.ReadAll(io.LimitReader(r.Body, 6*1024*1024+1))
		if e != nil || len(body) > 6*1024*1024 {
			http.Error(w, "fixture body limit", 413)
			return
		}
		var req struct {
			Messages []openrouter.Message `json:"messages"`
			Format   any                  `json:"response_format"`
		}
		if json.Unmarshal(body, &req) != nil {
			http.Error(w, "bad fixture request", 400)
			return
		}
		response := map[string]any{"usage": map[string]any{"cost": 0, "prompt_tokens": 0, "completion_tokens": 0}}
		var message any = map[string]any{"role": "assistant", "content": "Saved."}
		if req.Format != nil {
			var data struct {
				Items []grocery.Item `json:"items"`
			}
			text, _ := req.Messages[len(req.Messages)-1].Content.(string)
			json.Unmarshal([]byte(text), &data)
			ids := []int64{}
			for n := len(data.Items) - 1; n >= 0; n-- {
				ids = append(ids, data.Items[n].ID)
			}
			raw, _ := json.Marshal(map[string]any{"ordered_ids": ids})
			message = map[string]any{"role": "assistant", "content": string(raw)}
		} else if req.Messages[len(req.Messages)-1].Role == "user" {
			if _, ok := req.Messages[len(req.Messages)-1].Content.([]any); ok {
				w.Header().Set("Content-Type", "application/json")
				w.Write(photoResponse)
				return
			}
			args := `{"items":[{"name":"milk","category":"Dairy","quantity":1}]}`
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": "fixture", "type": "function", "function": map[string]any{"name": "add_items", "arguments": args}}}}
		}
		response["choices"] = []any{map[string]any{"finish_reason": "stop", "message": message}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir, e := os.MkdirTemp("/data", "workload-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	s, e := sqlite.Open(ctx, filepath.Join(dir, "fixture.db"))
	if e != nil {
		return e
	}
	defer s.Close()
	s.Bootstrap(ctx, []int64{-123})
	s.SetSetting(ctx, "llm.enabled", json.RawMessage(`true`))
	client := openrouter.New("fixture", srv.Client())
	client.URL = srv.URL
	client.AccountURL = srv.URL
	t := &transport{photo: photo}
	b := bot.New(s, t, client, "fixture")
	var id int64
	accept := func(text string) error {
		id++
		return s.Accept(ctx, []grocery.Update{{ID: id, ChatID: -123, UserID: 8, Text: text}}, id+1)
	}
	for n := 0; n < 60; n++ {
		if e = accept("/add Fixture item " + strconv.Itoa(n)); e != nil {
			return e
		}
	}
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	wait := func() error {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			complete, e := s.Completed(ctx, id)
			if e != nil {
				return e
			}
			if complete {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
		return errors.New("fixture processing timed out")
	}
	if e = wait(); e != nil {
		return e
	}
	for _, text := range []string{"/save usual", "/templates", "/alias milk = Fixture oat milk", "add milk", "/list", "/done 1 2", "/undo", "/remove 3", "/usage", "/usual"} {
		if e = accept(text); e != nil {
			return e
		}
		if e = wait(); e != nil {
			return e
		}
	}
	confirm := func() error {
		p, e := s.PendingAction(ctx, -123, 8)
		if e != nil {
			return e
		}
		id++
		u := grocery.Update{ID: id, ChatID: -123, UserID: 8, CallbackID: "fixture", CallbackData: sign("confirm", p.ID, 8)}
		if e = s.Accept(ctx, []grocery.Update{u}, id+1); e != nil {
			return e
		}
		return wait()
	}
	if e = confirm(); e != nil {
		return e
	}
	id++
	u := grocery.Update{ID: id, ChatID: -123, UserID: 8, Text: "Add the items from this photo.", PhotoID: "fixture"}
	if e = s.Accept(ctx, []grocery.Update{u}, id+1); e != nil {
		return e
	}
	if e = wait(); e != nil {
		return e
	}
	if e = confirm(); e != nil {
		return e
	}
	if e = accept("/clear"); e != nil {
		return e
	}
	if e = wait(); e != nil {
		return e
	}
	if e = confirm(); e != nil {
		return e
	}
	if e = accept("/undo"); e != nil {
		return e
	}
	if e = wait(); e != nil {
		return e
	}
	pending, e := s.Pending(ctx, -123)
	if e != nil || len(pending) != 123 {
		return fmt.Errorf("fixture invariant failed: pending=%d", len(pending))
	}
	if e = s.Prune(ctx, time.Now()); e != nil {
		return e
	}
	recent, e := s.Recent(ctx, -123, 100)
	if e != nil || len(recent) > 20 {
		return errors.New("fixture history not bounded")
	}
	if e = s.Check(ctx); e != nil {
		return e
	}
	time.Sleep(500 * time.Millisecond)
	cancel()
	if e = <-done; e != nil {
		return e
	}
	var usage unix.Rusage
	if e = unix.Getrusage(unix.RUSAGE_SELF, &usage); e != nil {
		return e
	}
	result := map[string]any{"updates_processed": id, "pending_fixture_items": len(pending), "retained_messages": len(recent), "fixture_http_requests": requests.Load(), "actual_openrouter_requests": 0, "photo_buffer_bytes": 4194304, "peak_rss_mib": float64(usage.Maxrss) / 1024, "delivered_fixture_messages": t.delivered.Load()}
	raw, _ := json.Marshal(result)
	fmt.Println(string(bytes.TrimSpace(raw)))
	return nil
}
