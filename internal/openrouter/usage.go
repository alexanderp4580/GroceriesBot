package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// AccountUsage exposes billing amounts only; labels, hashes and user IDs never enter bot replies.
type AccountUsage struct {
	Total, Daily, Monthly                    float64
	KeyLimit, KeyRemaining, CreditsRemaining *float64
}

func (c *Client) accountGET(ctx context.Context, path string) (json.RawMessage, int, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.AccountURL, "/")+path, nil)
	if e != nil {
		return nil, 0, errors.New("invalid usage endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, 0, errors.New("usage connection unavailable")
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if e != nil || len(raw) > 64*1024 {
		return nil, resp.StatusCode, errors.New("invalid usage response")
	}
	return raw, resp.StatusCode, nil
}
func (c *Client) Usage(ctx context.Context) (AccountUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, status, e := c.accountGET(ctx, "/key")
	if e != nil || status != 200 {
		return AccountUsage{}, errors.New("live API-key usage unavailable")
	}
	// Field mapping follows the recorded /key response; nullable limits stay nullable.
	var wire struct {
		Data struct {
			Usage     *float64 `json:"usage"`
			Daily     *float64 `json:"usage_daily"`
			Monthly   *float64 `json:"usage_monthly"`
			Limit     *float64 `json:"limit"`
			Remaining *float64 `json:"limit_remaining"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &wire) != nil || wire.Data.Usage == nil || wire.Data.Daily == nil || wire.Data.Monthly == nil {
		return AccountUsage{}, errors.New("invalid API-key usage response")
	}
	u := AccountUsage{Total: *wire.Data.Usage, Daily: *wire.Data.Daily, Monthly: *wire.Data.Monthly, KeyLimit: wire.Data.Limit, KeyRemaining: wire.Data.Remaining}
	if u.Total < 0 || u.Daily < 0 || u.Monthly < 0 {
		return AccountUsage{}, errors.New("invalid API-key usage amounts")
	}
	raw, status, e = c.accountGET(ctx, "/credits")
	if e != nil || status != 200 {
		return u, nil
	}
	var credit struct {
		Data struct {
			Purchased *float64 `json:"total_credits"`
			Used      *float64 `json:"total_usage"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &credit) == nil && credit.Data.Purchased != nil && credit.Data.Used != nil && *credit.Data.Purchased >= 0 && *credit.Data.Used >= 0 {
		remaining := *credit.Data.Purchased - *credit.Data.Used
		u.CreditsRemaining = &remaining
	}
	return u, nil
}
