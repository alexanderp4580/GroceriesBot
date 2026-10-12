package config

import (
	"encoding/json"
	"errors"
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TelegramToken string
	OpenRouterKey string
	DatabasePath  string
	RunMode       string
	Listen        string
	AllowedChats  []int64
}

func Load(file string) (Config, error) {
	values := map[string]string{}
	if file != "" {
		v, e := godotenv.Read(file)
		if e != nil && !os.IsNotExist(e) {
			return Config{}, errors.New("cannot read env file")
		}
		values = v
	}
	get := func(k, def string) string {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
		if v, ok := values[k]; ok {
			return v
		}
		return def
	}
	c := Config{TelegramToken: get("TELEGRAM_BOT_TOKEN", ""), OpenRouterKey: get("OPENROUTER_API_KEY", ""), DatabasePath: get("DATABASE_PATH", "./data/groceriesbot.sqlite3"), RunMode: get("RUN_MODE", "paused"), Listen: get("LISTEN_ADDR", "127.0.0.1:8080")}
	if c.RunMode != "active" && c.RunMode != "paused" {
		return Config{}, errors.New("RUN_MODE must be active or paused")
	}
	for _, s := range strings.Split(get("ALLOWED_CHAT_IDS", ""), ",") {
		if strings.TrimSpace(s) == "" {
			continue
		}
		id, e := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if e != nil || id == 0 {
			return Config{}, errors.New("invalid chat allowlist")
		}
		c.AllowedChats = append(c.AllowedChats, id)
	}
	return c, nil
}

type Settings map[string]json.RawMessage

func (s Settings) Int(k string) int          { var v int; json.Unmarshal(s[k], &v); return v }
func (s Settings) Float(k string) float64    { var v float64; json.Unmarshal(s[k], &v); return v }
func (s Settings) Bool(k string) bool        { var v bool; json.Unmarshal(s[k], &v); return v }
func (s Settings) String(k string) string    { var v string; json.Unmarshal(s[k], &v); return v }
func (s Settings) Strings(k string) []string { var v []string; json.Unmarshal(s[k], &v); return v }
func (s Settings) IDs(k string) []int64      { var v []int64; json.Unmarshal(s[k], &v); return v }
func (s Settings) Allowed(chat, user int64) bool {
	ok := false
	for _, id := range s.IDs("telegram.allowed_chat_ids") {
		if id == chat {
			ok = true
		}
	}
	if !ok {
		return false
	}
	users := s.IDs("telegram.allowed_user_ids")
	if len(users) == 0 {
		return true
	}
	for _, id := range users {
		if id == user {
			return true
		}
	}
	return false
}
