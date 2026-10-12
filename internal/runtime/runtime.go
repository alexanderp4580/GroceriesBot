package runtime

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"os"
	"time"
)

func AcquireLock(path string) (io.Closer, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("another GroceriesBot instance holds the data lock")
	}
	return f, nil
}
func HealthHandler(check func(context.Context) error, ready func() bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if check(ctx) != nil || r.URL.Path == "/readyz" && !ready() {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok\n"))
	})
}
