package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type fixtureTransport struct {
	Calls int
	Raw   string
}

func (f *fixtureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.Calls++
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(f.Raw)), Header: make(http.Header)}, nil
}
func TestRecorderKeepsUnknownResponseShape(t *testing.T) {
	base := &fixtureTransport{Raw: `{"future_response":{"unexpected":[1,2]},"usage":{"cost":0.0001}}`}
	r := &recorder{Base: base, Path: t.TempDir(), Budget: .25, Prompt: .000001, Completion: .000001, Model: "fixture"}
	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBufferString(`{"model":"fixture","max_tokens":100}`))
	response, e := r.RoundTrip(req)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	raw, e := os.ReadFile(filepath.Join(r.Path, "request-001.json"))
	var entry map[string]json.RawMessage
	json.Unmarshal(raw, &entry)
	var actual map[string]any
	json.Unmarshal(entry["response"], &actual)
	if e != nil || actual["future_response"] == nil || base.Calls != 1 {
		t.Fatal(string(raw), e)
	}
}
func TestRecorderBudgetRejectsBeforeProvider(t *testing.T) {
	base := &fixtureTransport{Raw: `{}`}
	r := &recorder{Base: base, Path: t.TempDir(), Budget: .001, Prompt: 1, Completion: 1, Model: "fixture"}
	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBufferString(`{"model":"fixture","max_tokens":100}`))
	if _, e := r.RoundTrip(req); e == nil || base.Calls != 0 {
		t.Fatal("provider called above capture budget")
	}
}
