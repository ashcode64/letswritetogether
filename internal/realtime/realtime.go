package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/redis/go-redis/v9"

	"colorbook/internal/keys"
)

type client struct {
	ch   chan string
	drop chan struct{}
	once sync.Once
}

func (c *client) kill() { c.once.Do(func() { close(c.drop) }) }

// Hub fans Redis pub/sub out to SSE clients and drops a slow one.
type Hub struct {
	RDB *redis.Client
	mu  sync.Mutex
	all map[*client]struct{}
}

func New(rdb *redis.Client) *Hub {
	return &Hub{RDB: rdb, all: map[*client]struct{}{}}
}

func (h *Hub) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/events", h.events)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (h *Hub) Listen(ctx context.Context) {
	sub := h.RDB.Subscribe(ctx, keys.Channel)
	for msg := range sub.Channel() {
		h.broadcast(msg.Payload)
	}
}

func (h *Hub) broadcast(payload string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.all {
		select {
		case c.ch <- payload:
		default:
			c.kill()
		}
	}
}

func (h *Hub) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	c := &client{ch: make(chan string, 32), drop: make(chan struct{})}
	h.mu.Lock()
	h.all[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.all, c)
		h.mu.Unlock()
	}()
	fmt.Fprintf(w, ": ok\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.drop:
			return
		case msg := <-c.ch:
			var env struct {
				Type string `json:"type"`
			}
			if json.Unmarshal([]byte(msg), &env) != nil || env.Type == "" {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", env.Type, msg)
			fl.Flush()
		}
	}
}
