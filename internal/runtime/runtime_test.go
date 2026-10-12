package runtime

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestLifetimeLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	one, e := AcquireLock(p)
	if e != nil {
		t.Fatal(e)
	}
	if two, e := AcquireLock(p); e == nil {
		two.Close()
		t.Fatal("overlapping instance")
	}
	one.Close()
	two, e := AcquireLock(p)
	if e != nil {
		t.Fatal(e)
	}
	two.Close()
}
func TestHealthAndPausedReadiness(t *testing.T) {
	h := HealthHandler(func(context.Context) error { return nil }, func() bool { return false })
	for path, code := range map[string]int{"/healthz": 200, "/readyz": 503, "/missing": 404} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != code {
			t.Fatal(path, r.Code)
		}
	}
	h = HealthHandler(func(context.Context) error { return errors.New("storage") }, func() bool { return true })
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/healthz", nil))
	if r.Code != 503 {
		t.Fatal("storage outage healthy")
	}
}
