package telegram

import (
	"context"
	"encoding/json"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPollingAndCallbackAndReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "getUpdates") {
			w.Write([]byte(`{"ok":true,"result":[{"update_id":5,"callback_query":{"id":"cb","from":{"id":8},"data":"buy:9","message":{"message_id":20,"chat":{"id":-123}}}}]}`))
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["chat_id"] != float64(-123) {
			t.Error("wrong chat")
		}
		w.Write([]byte(`{"ok":true,"result":{"message_id":21}}`))
	}))
	defer srv.Close()
	c := New("fixture", srv.Client())
	c.BaseURL = srv.URL
	us, e := c.Poll(context.Background(), 0, 1)
	if e != nil || len(us) != 1 || us[0].CallbackData != "buy:9" || us[0].UserID != 8 {
		t.Fatal(us, e)
	}
	id, e := c.Deliver(context.Background(), grocery.Outgoing{Method: "sendMessage", ChatID: -123, Text: "fixture"})
	if e != nil || id != 21 {
		t.Fatal(id, e)
	}
}
func TestPhotoReadIsBoundedWithoutContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "getFile") {
			w.Write([]byte(`{"ok":true,"result":{"file_path":"photo.jpg"}}`))
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.(http.Flusher).Flush()
		w.Write([]byte(strings.Repeat("x", 2048)))
	}))
	defer srv.Close()
	c := New("fixture", srv.Client())
	c.BaseURL = srv.URL
	c.FileURL = srv.URL
	if _, _, e := c.Download(context.Background(), "one-photo", 1024); e == nil {
		t.Fatal("accepted oversized image")
	}
}
