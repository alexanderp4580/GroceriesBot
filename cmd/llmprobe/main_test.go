package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"github.com/alexanderp4580/GroceriesBot/internal/sqlite"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// The recorded photo used ask/clarify, not confirm_add. Follow the actual button label.
func TestFollowUpAnswersCapturedClarification(t *testing.T) {
	ctx := context.Background()
	s, e := sqlite.Open(ctx, filepath.Join(t.TempDir(), "fixture.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.Accept(ctx, []grocery.Update{{ID: 1, ChatID: -123, UserID: 8}}, 2)
	p, e := s.Prepare(ctx, grocery.ActionKey{UpdateID: 1}, -123, 8, "clarify", map[string]any{"confirm_required": true, "original": "Add groceries from this note.", "question": "Shall I add these 3 items?", "options": []string{"Yes, add all 3", "No, cancel"}}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	u, _, skip, e := followUp(ctx, s, step{Confirm: true, OptionText: "Yes, add all 3", Fallback: "Yes, add all three items."}, 2)
	if e != nil || skip || u.CallbackData != callback("choice0", p.ID) {
		t.Fatal(u, skip, e)
	}
}
func TestFollowUpAnswersTextOnlyProposal(t *testing.T) {
	ctx := context.Background()
	s, e := sqlite.Open(ctx, filepath.Join(t.TempDir(), "fixture.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	u, _, skip, e := followUp(ctx, s, step{Confirm: true, Fallback: "Yes, add all the proposed ingredients."}, 2)
	if e != nil || skip || u.Text != "Yes, add all the proposed ingredients." || u.CallbackData != "" {
		t.Fatal(u, skip, e)
	}
}
func TestScenarioSelectionDoesNotRepeatUnselectedCases(t *testing.T) {
	cases, e := selectCases([]scenario{{ID: "photo"}, {ID: "ingredients"}, {ID: "explicit_add"}}, []string{"photo", "ingredients"})
	if e != nil || len(cases) != 2 {
		t.Fatal(cases, e)
	}
	if _, e = selectCases([]scenario{{ID: "photo"}}, []string{"typo"}); e == nil {
		t.Fatal("unknown selection accepted")
	}
}
