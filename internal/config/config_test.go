package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKeepsProcessValuesAndRejectsBadMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("TELEGRAM_BOT_TOKEN=from-file\nDATABASE_PATH=./fixture.db\nRUN_MODE=paused\n"), 0600)
	t.Setenv("TELEGRAM_BOT_TOKEN", "process-token")
	c, err := Load(p)
	if err != nil || c.TelegramToken != "process-token" || c.RunMode != "paused" {
		t.Fatalf("config: %+v %v", c, err)
	}
	t.Setenv("RUN_MODE", "unknown")
	if _, err = Load(p); err == nil {
		t.Fatal("accepted invalid mode")
	}
}
