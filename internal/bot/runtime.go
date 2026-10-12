package bot

import (
	"context"
	"errors"
	"github.com/alexanderp4580/GroceriesBot/internal/sqlite"
	"github.com/alexanderp4580/GroceriesBot/internal/telegram"
	"log/slog"
	"sync"
	"time"
)

func (b *Bot) Run(ctx context.Context) error {
	if e := b.db.Recover(ctx); e != nil {
		return e
	}
	if e := b.db.Prune(ctx, time.Now()); e != nil {
		return e
	}
	var wg sync.WaitGroup
	wake := make(chan struct{}, 1)
	notify := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	wg.Add(3)
	go func() {
		defer wg.Done()
		backoff := time.Second
		for ctx.Err() == nil {
			offset, e := b.db.Offset(ctx)
			if e != nil {
				slog.Error("poll offset unavailable")
				if !wait(ctx, time.Second) {
					return
				}
				continue
			}
			cfg, e := b.db.Settings(ctx)
			if e != nil {
				if !wait(ctx, time.Second) {
					return
				}
				continue
			}
			us, e := b.telegram.Poll(ctx, offset, cfg.Int("telegram.poll_timeout_seconds"))
			if e != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("Telegram poll failed")
				if !wait(ctx, backoff) {
					return
				}
				backoff = min(backoff*2, 30*time.Second)
				continue
			}
			backoff = time.Second
			if len(us) == 0 {
				continue
			}
			for _, u := range us {
				offset = max(offset, u.ID+1)
			}
			if e = b.db.Accept(ctx, us, offset); e != nil {
				slog.Error("update intake failed")
				if !wait(ctx, time.Second) {
					return
				}
				continue
			}
			notify()
		}
	}()
	go func() {
		defer wg.Done()
		tick := time.NewTicker(time.Second)
		prune := time.NewTicker(time.Hour)
		defer tick.Stop()
		defer prune.Stop()
		for ctx.Err() == nil {
			for ctx.Err() == nil {
				u, e := b.db.NextUpdate(ctx)
				if sqlite.IsEmpty(e) {
					break
				}
				if e != nil {
					slog.Error("update read failed")
					break
				}
				if e = b.Process(ctx, u); e != nil {
					slog.Error("update completion failed", "update_id", u.ID)
					b.db.Recover(ctx)
					if !wait(ctx, time.Second) {
						return
					}
					break
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-wake:
			case <-tick.C:
			case <-prune.C:
				if e := b.db.Prune(ctx, time.Now()); e != nil {
					slog.Error("retention failed")
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			r, e := b.db.NextReply(ctx)
			if sqlite.IsEmpty(e) {
				if !wait(ctx, 250*time.Millisecond) {
					return
				}
				continue
			}
			if e != nil {
				if !wait(ctx, time.Second) {
					return
				}
				continue
			}
			id, e := b.telegram.Deliver(ctx, r.Message)
			if ctx.Err() != nil {
				return
			}
			if e == nil {
				if e = b.db.Sent(ctx, r, id); e != nil {
					slog.Error("delivery checkpoint failed")
				}
				continue
			}
			delay := time.Duration(1<<min(r.Attempts, 5)) * time.Second
			permanent := false
			var ae *telegram.APIError
			if errors.As(e, &ae) {
				permanent = ae.Permanent
				delay = max(delay, time.Duration(ae.RetryAfter)*time.Second)
				if ae.Code == 400 && r.Message.Method == "editMessageText" {
					permanent = true
				}
			}
			if e = b.db.RetryReply(ctx, r, delay, permanent); e != nil {
				slog.Error("retry checkpoint failed")
			}
			slog.Warn("Telegram delivery failed", "outbox_id", r.ID, "permanent", permanent)
		}
	}()
	<-ctx.Done()
	wg.Wait()
	return nil
}
func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
