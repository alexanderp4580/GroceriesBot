// llmprobe records real provider traffic while exercising the bot on synthetic data.
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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type configuration struct {
	Key, Record, Model                   string
	MaxUSD, PromptPrice, CompletionPrice float64
	Only                                 []string
	Resume                               bool
	StartIDs                             map[string]int64
}
type step struct {
	Text       string `json:"text"`
	OptionText string `json:"option_text"`
	Reply      string `json:"reply_to"`
	Photo      string `json:"photo"`
	Option     *int   `json:"select_option"`
	Fallback   string `json:"fallback_text"`
	Confirm    bool   `json:"confirm"`
}
type scenario struct {
	ID      string         `json:"id"`
	Find    string         `json:"find_out"`
	Pending []grocery.Item `json:"pending"`
	History []grocery.Item `json:"history"`
	Steps   []step         `json:"steps"`
}
type recorder struct {
	Base                             http.RoundTripper
	Path, Model, Scenario            string
	Budget, Used, Prompt, Completion float64
	Count                            int
}

func (r *recorder) write(n int, v any) error {
	raw, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(r.Path, fmt.Sprintf("request-%03d.json", n)), raw, 0600)
}
func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	raw, e := io.ReadAll(io.LimitReader(req.Body, 6*1024*1024+1))
	if e != nil || len(raw) > 6*1024*1024 {
		return nil, errors.New("capture request exceeds limit")
	}
	var body struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Model != r.Model || req.URL.Host != "openrouter.ai" {
		return nil, errors.New("unexpected capture request")
	}
	inputBound, e := captureInputBound(raw)
	if e != nil {
		return nil, e
	}
	allowance := float64(inputBound)*r.Prompt + float64(body.MaxTokens)*r.Completion
	if r.Used+allowance > r.Budget {
		return nil, errors.New("capture budget exhausted")
	}
	r.Used += allowance
	r.Count++
	n := r.Count
	entry := map[string]any{"scenario": r.Scenario, "request": json.RawMessage(raw), "reserved_usd": allowance}
	if e = r.write(n, entry); e != nil {
		return nil, errors.New("cannot persist capture request")
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	start := time.Now()
	resp, e := r.Base.RoundTrip(req)
	entry["latency_ms"] = time.Since(start).Milliseconds()
	if e != nil {
		entry["transport_error"] = "request outcome unknown"
		r.write(n, entry)
		return nil, errors.New("capture connection failed")
	}
	response, e := io.ReadAll(io.LimitReader(resp.Body, 512*1024+1))
	resp.Body.Close()
	entry["status"] = resp.StatusCode
	if json.Valid(response) {
		entry["response"] = json.RawMessage(response)
	} else {
		entry["response_text"] = string(response)
	}
	if len(response) > 512*1024 || e != nil {
		entry["read_error"] = "response exceeds limit or cannot be read"
		r.write(n, entry)
		return nil, errors.New("capture response read failed")
	}
	var usage struct {
		Usage struct {
			Cost *float64 `json:"cost"`
		} `json:"usage"`
	}
	if json.Unmarshal(response, &usage) == nil && usage.Usage.Cost != nil && *usage.Usage.Cost >= 0 {
		r.Used += *usage.Usage.Cost - allowance
		entry["recorded_cost_usd"] = *usage.Usage.Cost
	}
	if e = r.write(n, entry); e != nil {
		return nil, errors.New("cannot persist capture response")
	}
	resp.Body = io.NopCloser(bytes.NewReader(response))
	return resp, nil
}

type messenger struct{ Photo []byte }

func (m messenger) Poll(context.Context, int64, int) ([]grocery.Update, error) { return nil, nil }
func (m messenger) Deliver(context.Context, grocery.Outgoing) (int64, error)   { return 1, nil }
func (m messenger) Download(context.Context, string, int64) ([]byte, string, error) {
	return m.Photo, "image/png", nil
}
func callback(kind string, id int64) string {
	body := fmt.Sprintf("gb:%s:%d", kind, id)
	h := hmac.New(sha256.New, []byte("synthetic-capture-signing-key"))
	fmt.Fprintf(h, "%s:%d:%d", body, int64(-123), int64(8))
	return body + ":" + base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:10])
}
func observe(ctx context.Context, s *sqlite.Store) map[string]any {
	p, _ := s.Pending(ctx, -123)
	h, _ := s.History(ctx, -123)
	pending, e := s.PendingAction(ctx, -123, 8)
	v := map[string]any{"pending": p, "history": h}
	if e == nil {
		v["question_or_confirmation"] = pending
	}
	return v
}
func seed(ctx context.Context, s *sqlite.Store, id *int64, items []grocery.Item, purchased bool) error {
	if len(items) == 0 {
		return nil
	}
	*id++
	if e := s.Accept(ctx, []grocery.Update{{ID: *id, ChatID: -123, UserID: 8}}, *id+1); e != nil {
		return e
	}
	result, e := s.Apply(ctx, grocery.ActionKey{UpdateID: *id}, grocery.Operation{Kind: "add", ChatID: -123, UserID: 8, Items: items}, time.Now())
	if e != nil {
		return e
	}
	if purchased {
		*id++
		s.Accept(ctx, []grocery.Update{{ID: *id, ChatID: -123, UserID: 8}}, *id+1)
		ids := []int64{}
		for _, i := range result.Items {
			ids = append(ids, i.ID)
		}
		_, e = s.Apply(ctx, grocery.ActionKey{UpdateID: *id}, grocery.Operation{Kind: "purchase", ChatID: -123, UserID: 8, IDs: ids}, time.Now())
	}
	return e
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(1)
	}
}
func run() error {
	var cfg configuration
	if json.NewDecoder(os.Stdin).Decode(&cfg) != nil || cfg.Key == "" || cfg.Record == "" || cfg.MaxUSD <= 0 || cfg.PromptPrice <= 0 || cfg.CompletionPrice <= 0 {
		return errors.New("capture configuration missing")
	}
	raw, e := os.ReadFile("testdata/llm-scenarios.json")
	if e != nil {
		return e
	}
	var cases []scenario
	if e = json.Unmarshal(raw, &cases); e != nil {
		return e
	}
	cases, e = selectCases(cases, cfg.Only)
	if e != nil {
		return e
	}
	photo, e := os.ReadFile(filepath.Join(cfg.Record, "grocery-note.png"))
	if e != nil {
		return e
	}
	r := &recorder{Base: http.DefaultTransport, Path: cfg.Record, Budget: cfg.MaxUSD, Prompt: cfg.PromptPrice, Completion: cfg.CompletionPrice, Model: cfg.Model}
	client := openrouter.New(cfg.Key, &http.Client{Transport: r, Timeout: 35 * time.Second})
	results := []map[string]any{}
	for _, c := range cases {
		ctx := context.Background()
		r.Scenario = c.ID
		s, e := sqlite.Open(ctx, filepath.Join(cfg.Record, c.ID+".sqlite3"))
		if e != nil {
			return e
		}
		s.Bootstrap(ctx, []int64{-123})
		s.SetSetting(ctx, "llm.enabled", json.RawMessage(`true`))
		modelJSON, _ := json.Marshal(cfg.Model)
		s.SetSetting(ctx, "llm.model", modelJSON)
		budgetJSON, _ := json.Marshal(cfg.MaxUSD)
		s.SetSetting(ctx, "llm.daily_budget_usd", budgetJSON)
		s.SetSetting(ctx, "llm.monthly_budget_usd", budgetJSON)
		var id int64
		if cfg.Resume {
			id = cfg.StartIDs[c.ID]
			if id == 0 {
				return errors.New("resume source update ID missing")
			}
			if len(c.Steps) > 0 {
				c.Steps = c.Steps[1:]
			}
		} else {
			if e = seed(ctx, s, &id, c.History, true); e != nil {
				return e
			}
			if e = seed(ctx, s, &id, c.Pending, false); e != nil {
				return e
			}
		}

		b := bot.New(s, messenger{photo}, client, "synthetic-capture-signing-key")
		result := map[string]any{"scenario": c.ID, "find_out": c.Find, "before": observe(ctx, s)}
		observations := []map[string]any{}
		for _, step := range c.Steps {
			id++
			u, note, skip, e := followUp(ctx, s, step, id)
			if e != nil {
				return e
			}
			if skip {
				observations = append(observations, map[string]any{"note": note, "state": observe(ctx, s)})
				continue
			}

			if e = s.Accept(ctx, []grocery.Update{u}, id+1); e != nil {
				return e
			}
			e = b.Process(ctx, u)
			o := map[string]any{"input": u, "note": note, "state": observe(ctx, s)}
			if e != nil {
				o["processor_error"] = "processing failed"
			}
			messages := []grocery.Outgoing{}
			for {
				reply, e := s.NextReply(ctx)
				if sqlite.IsEmpty(e) {
					break
				}
				if e != nil {
					return e
				}
				messages = append(messages, reply.Message)
				s.Sent(ctx, reply, 0)
			}
			o["bot_messages"] = messages
			observations = append(observations, o)
		}
		result["steps"] = observations
		result["after"] = observe(ctx, s)
		results = append(results, result)
		s.Close()
		out, _ := json.MarshalIndent(map[string]any{"scenarios": results, "http_requests": r.Count, "spent_or_reserved_usd": r.Used, "model": cfg.Model, "effort": "medium"}, "", "  ")
		if e = os.WriteFile(filepath.Join(cfg.Record, "observations.json"), out, 0600); e != nil {
			return e
		}
		fmt.Println(c.ID + ": recorded")
		if r.Used >= r.Budget {
			break
		}
	}
	return nil
}
