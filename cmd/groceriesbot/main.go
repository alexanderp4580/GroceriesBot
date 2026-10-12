package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/bot"
	"github.com/alexanderp4580/GroceriesBot/internal/config"
	"github.com/alexanderp4580/GroceriesBot/internal/openrouter"
	life "github.com/alexanderp4580/GroceriesBot/internal/runtime"
	"github.com/alexanderp4580/GroceriesBot/internal/sqlite"
	"github.com/alexanderp4580/GroceriesBot/internal/telegram"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		slog.Error("GroceriesBot stopped", "error", e.Error())
		os.Exit(1)
	}
}
func run() error {
	cfg, e := config.Load(".env")
	if e != nil {
		return e
	}
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "healthcheck" {
		client := &http.Client{Timeout: 5 * time.Second}
		r, e := client.Get("http://" + cfg.Listen + "/healthz")
		if e != nil {
			return errors.New("health probe failed")
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			return errors.New("unhealthy")
		}
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var lock interface{ Close() error }
	if len(args) == 0 {
		if e = os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0700); e != nil {
			return e
		}
		lock, e = life.AcquireLock(filepath.Join(filepath.Dir(cfg.DatabasePath), "groceriesbot.lock"))
		if e != nil {
			return e
		}
		defer lock.Close()
	}
	db, e := sqlite.Open(ctx, cfg.DatabasePath)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = db.Check(ctx); e != nil {
		return e
	}
	if e = db.Bootstrap(ctx, cfg.AllowedChats); e != nil {
		return e
	}
	if len(args) > 0 {
		switch args[0] {
		case "doctor":
			fmt.Println("Database integrity, relationships and migrations: ok")
			return nil
		case "settings":
			if len(args) == 2 && args[1] == "list" {
				v, e := db.Settings(ctx)
				if e != nil {
					return e
				}
				return json.NewEncoder(os.Stdout).Encode(v)
			}
			if len(args) == 3 && args[1] == "get" {
				v, e := db.Settings(ctx)
				if e != nil {
					return e
				}
				x, ok := v[args[2]]
				if !ok {
					return errors.New("unknown setting")
				}
				fmt.Println(string(x))
				return nil
			}
			if len(args) == 4 && args[1] == "set" {
				return db.SetSetting(ctx, args[2], json.RawMessage(args[3]))
			}
		case "webhook":
			t := telegram.New(cfg.TelegramToken, telegram.HTTPClient())
			if len(args) == 2 && args[1] == "remove" {
				return t.RemoveWebhook(ctx)
			}
		}
		return errors.New("usage: groceriesbot [doctor|healthcheck|settings list|get KEY|set KEY JSON|webhook remove]")
	}
	ready := false
	t := telegram.New(cfg.TelegramToken, telegram.HTTPClient())
	if cfg.RunMode == "active" {
		v, e := db.Settings(ctx)
		if e != nil {
			return e
		}
		if len(v.IDs("telegram.allowed_chat_ids")) == 0 || cfg.TelegramToken == "" {
			return errors.New("active mode requires a bot token and chat allowlist")
		}
		webhook, e := t.Webhook(ctx)
		if e != nil {
			return e
		}
		var info struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(webhook, &info) != nil {
			return errors.New("invalid webhook information")
		}
		if info.URL != "" {
			return errors.New("Telegram webhook exists; complete explicit cutover before polling")
		}
		ready = true
	}
	srv := &http.Server{Addr: cfg.Listen, Handler: life.HealthHandler(db.Ping, func() bool { return ready }), ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() {
		e := srv.ListenAndServe()
		if !errors.Is(e, http.ErrServerClosed) {
			errc <- e
		}
		stop()
	}()
	slog.Info("GroceriesBot started", "mode", cfg.RunMode)
	botDone := make(chan struct{})
	if ready {
		m := openrouter.New(cfg.OpenRouterKey, telegram.HTTPClient())
		go func() {
			defer close(botDone)
			if e := bot.New(db, t, m, cfg.TelegramToken).Run(ctx); e != nil {
				errc <- e
				stop()
			}
		}()
	} else {
		close(botDone)
	}
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(shutdown)
	select {
	case <-botDone:
	case <-shutdown.Done():
		return errors.New("shutdown timed out")
	}
	select {
	case e := <-errc:
		return e
	default:
		return nil
	}
}
