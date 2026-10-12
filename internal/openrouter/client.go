package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}
type Message struct {
	Role             string            `json:"role"`
	Content          any               `json:"content"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
	ReasoningDetails []json.RawMessage `json:"reasoning_details,omitempty"`
}
type Tool struct {
	Type     string         `json:"type"`
	Function map[string]any `json:"function"`
}
type Request struct {
	Model     string
	Messages  []Message
	Tools     []Tool
	MaxTokens int
	Effort    string
	JSON      bool
}
type Usage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
	Details    struct {
		Reasoning int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	Cost      float64 `json:"cost"`
	Reasoning int     `json:"-"`
	LatencyMS int64   `json:"-"`
}
type Response struct {
	Message Message `json:"message"`
	Usage   Usage   `json:"usage"`
}
type Client struct {
	key  string
	HTTP *http.Client
	URL  string
}

func New(key string, h *http.Client) *Client {
	return &Client{key: key, HTTP: h, URL: "https://openrouter.ai/api/v1/chat/completions"}
}
func (c *Client) Complete(ctx context.Context, r Request) (Response, error) {
	body := map[string]any{"model": r.Model, "messages": r.Messages, "max_tokens": r.MaxTokens, "reasoning": map[string]any{"effort": r.Effort}, "provider": map[string]any{"order": []string{"Anthropic"}, "allow_fallbacks": false, "require_parameters": true}}
	if len(r.Tools) > 0 {
		body["tools"] = r.Tools
	}
	if r.JSON {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	b, e := json.Marshal(body)
	if e != nil {
		return Response{}, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(b))
	if e != nil {
		return Response{}, errors.New("invalid model request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("X-Title", "GroceriesBot")
	start := time.Now()
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return Response{}, errors.New("model connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Response{}, errors.New("model provider rejected request")
	}
	b, e = io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if e != nil || len(b) > 256*1024 {
		return Response{}, errors.New("model response exceeds limit")
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
			Finish  string  `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if json.Unmarshal(b, &out) != nil || len(out.Choices) != 1 {
		return Response{}, errors.New("invalid model response")
	}
	if out.Choices[0].Finish == "length" {
		return Response{}, errors.New("model output was truncated; no actions applied")
	}
	out.Usage.Reasoning = out.Usage.Details.Reasoning
	out.Usage.LatencyMS = time.Since(start).Milliseconds()
	if out.Usage.Cost == 0 {
		out.Usage.Cost = (float64(out.Usage.Prompt)*0.1 + float64(out.Usage.Completion)*0.5) / 1e6
	}
	return Response{Message: out.Choices[0].Message, Usage: out.Usage}, nil
}
