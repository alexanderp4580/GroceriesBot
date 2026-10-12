package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"github.com/alexanderp4580/GroceriesBot/internal/sqlite"
)

func selectCases(cases []scenario, only []string) ([]scenario, error) {
	if len(only) == 0 {
		return cases, nil
	}
	wanted := map[string]bool{}
	for _, id := range only {
		wanted[id] = true
	}
	result := []scenario{}
	for _, c := range cases {
		if wanted[c.ID] {
			result = append(result, c)
			delete(wanted, c.ID)
		}
	}
	if len(wanted) > 0 {
		return nil, errors.New("unknown scenario selected")
	}
	return result, nil
}
func followUp(ctx context.Context, s *sqlite.Store, step step, id int64) (grocery.Update, string, bool, error) {
	u := grocery.Update{ID: id, ChatID: -123, UserID: 8, Text: step.Text, ReplyTo: step.Reply, PhotoID: step.Photo}
	if step.Option == nil && step.OptionText == "" && !step.Confirm {
		return u, "", false, nil
	}
	p, e := s.PendingAction(ctx, -123, 8)
	if e != nil && !sqlite.IsEmpty(e) {
		return u, "", false, e
	}
	if e == nil && step.Confirm && (p.Kind == "confirm_add" || p.Kind == "confirm_clear") {
		u.CallbackID = "fixture"
		u.CallbackData = callback("confirm", p.ID)
		return u, "", false, nil
	}
	if e == nil && p.Kind == "clarify" {
		var a struct {
			Options []string `json:"options"`
		}
		if e = json.Unmarshal(p.Payload, &a); e != nil {
			return u, "", false, e
		}
		option := -1
		if step.OptionText != "" {
			for n, label := range a.Options {
				if label == step.OptionText {
					option = n
					break
				}
			}
		} else if step.Option != nil {
			option = *step.Option
		}
		if option >= 0 && option < len(a.Options) {
			u.CallbackID = "fixture"
			u.CallbackData = callback(fmt.Sprintf("choice%d", option), p.ID)
			return u, "", false, nil
		}
		if step.Fallback != "" {
			u.Text = step.Fallback
			return u, "Answered the returned clarification with the configured user reply.", false, nil
		}
		return u, "No matching clarification option; no answer was invented.", true, nil
	}
	if step.Fallback != "" {
		u.Text = step.Fallback
		return u, "Sent the configured user reply to the text-only proposal.", false, nil
	}
	return u, "Requested follow-up interaction was absent.", true, nil
}
