package stream

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHubPublishAndClose(t *testing.T) {
	hub := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(hub)
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	lines := bufio.NewReader(res.Body)
	readFrame := func() string {
		var b strings.Builder
		for {
			line, err := lines.ReadString('\n')
			if err != nil {
				return b.String()
			}
			if line == "\n" {
				return b.String()
			}
			b.WriteString(line)
		}
	}

	if got := readFrame(); got != "retry: 3000\n" {
		t.Fatalf("first frame = %q", got)
	}

	// The client subscribed before the retry frame was flushed, so this reaches it.
	hub.Publish("score", []byte(`{"id":"x"}`))
	if got := readFrame(); got != "event: score\ndata: {\"id\":\"x\"}\n" {
		t.Fatalf("event frame = %q", got)
	}

	hub.Close()
	done := make(chan struct{})
	go func() { io.Copy(io.Discard, res.Body); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open after Close")
	}

	if res, err := http.Get(srv.URL); err != nil || res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("after Close: %v %v", res.StatusCode, err)
	}
}
