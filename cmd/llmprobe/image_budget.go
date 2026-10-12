package main

import (
	"encoding/json"
	"strings"
)

// Anthropic documents at most 4784 visual tokens per ordinary image on high-resolution models.
// Reserve 8192 per inline image, plus a byte-per-text-token bound and protocol overhead.
// Source: https://platform.claude.com/docs/en/build-with-claude/vision#resolution-and-token-cost
// Strip bytes only from the budget estimate; send and persist the original request unchanged.
func captureInputBound(raw []byte) (int, error) {
	var v any
	if e := json.Unmarshal(raw, &v); e != nil {
		return 0, e
	}
	images := 0
	var visit func(any)
	visit = func(value any) {
		switch x := value.(type) {
		case []any:
			for _, child := range x {
				visit(child)
			}
		case map[string]any:
			if x["type"] == "image_url" {
				if image, ok := x["image_url"].(map[string]any); ok {
					if url, ok := image["url"].(string); ok && strings.HasPrefix(url, "data:image/") {
						images++
						image["url"] = "inline image"
					}
				}
			}
			for _, child := range x {
				visit(child)
			}
		}
	}
	visit(v)
	text, e := json.Marshal(v)
	return len(text) + 4096 + images*8192, e
}
