package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolAndUsageDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "fixture" {
			t.Error("model")
		}
		w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call1","type":"function","function":{"name":"add_items","arguments":"{}"}}]}}],"usage":{"prompt_tokens":12,"completion_tokens":15,"completion_tokens_details":{"reasoning_tokens":8},"cost":0.00001}}`))
	}))
	defer srv.Close()
	c := New("fixture", srv.Client())
	c.URL = srv.URL
	r, e := c.Complete(context.Background(), Request{Model: "fixture", Messages: []Message{{Role: "user", Content: "milk"}}, MaxTokens: 4096, Effort: "medium"})
	if e != nil || len(r.Message.ToolCalls) != 1 || r.Usage.Reasoning != 8 {
		t.Fatal(r, e)
	}
}
func TestTruncatedCallsRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"partial"}}]}`))
	}))
	defer srv.Close()
	c := New("fixture", srv.Client())
	c.URL = srv.URL
	if _, e := c.Complete(context.Background(), Request{}); e == nil {
		t.Fatal("accepted truncated response")
	}
}
func TestMissingProviderCostIsNotInvented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":1000,"completion_tokens":500}}`))
	}))
	defer srv.Close()
	c := New("fixture", srv.Client())
	c.URL = srv.URL
	r, e := c.Complete(context.Background(), Request{})
	if e != nil || r.Usage.Cost != 0 || r.Usage.CostKnown {
		t.Fatal("invented usage cost", r, e)
	}
}
func TestUsageReadsKeyAndCreditsWithoutModelCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("usage must be read-only")
		}
		switch r.URL.Path {
		case "/key":
			w.Write([]byte(`{"data":{"usage":25.5,"usage_daily":1.25,"usage_monthly":3.75,"limit":100,"limit_remaining":74.5}}`))
		case "/credits":
			w.Write([]byte(`{"data":{"total_credits":100.5,"total_usage":25.75}}`))
		default:
			t.Error("unexpected model request")
			http.Error(w, "bad", 400)
		}
	}))
	defer srv.Close()
	c := New("fixture", srv.Client())
	c.AccountURL = srv.URL
	u, e := c.Usage(context.Background())
	if e != nil || u.Total != 25.5 || u.CreditsRemaining == nil || *u.CreditsRemaining != 74.75 || u.KeyRemaining == nil || *u.KeyRemaining != 74.5 {
		t.Fatal(u, e)
	}
}
func TestUsageDoesNotInventBalanceWhenCreditsRestricted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/credits" {
			http.Error(w, "forbidden", 403)
			return
		}
		w.Write([]byte(`{"data":{"usage":1,"usage_daily":0.1,"usage_monthly":0.3,"limit":null,"limit_remaining":null}}`))
	}))
	defer srv.Close()
	c := New("fixture", srv.Client())
	c.AccountURL = srv.URL
	u, e := c.Usage(context.Background())
	if e != nil || u.CreditsRemaining != nil || u.KeyRemaining != nil || u.Total != 1 {
		t.Fatal(u, e)
	}
}
