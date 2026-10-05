// gmeter-demo is a bounded local target that rejects excess work with HTTP 503.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	delay := flag.Duration("delay", 50*time.Millisecond, "work duration")
	capacity := flag.Int("capacity", 8, "concurrent work slots")
	flag.Parse()
	if *capacity < 1 || *delay < 0 {
		log.Fatal("capacity must be positive and delay must be nonnegative")
	}
	slots := make(chan struct{}, *capacity)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil || input.Username == "" || len(input.Username) > 128 {
			http.Error(w, "username required", 400)
			return
		}
		token := base64.RawURLEncoding.EncodeToString([]byte(input.Username))
		http.SetCookie(w, &http.Cookie{Name: "gmeter_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Demo-Session", token)
		json.NewEncoder(w).Encode(map[string]any{"token": token, "user": map[string]string{"id": input.Username}})
	})
	authenticated := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		cookie, err := r.Cookie("gmeter_session")
		if err != nil || cookie.Value == "" || r.Header.Get("Authorization") != "Bearer "+cookie.Value {
			http.Error(w, "session cookie and bearer token required", 401)
			return "", false
		}
		username, err := base64.RawURLEncoding.DecodeString(cookie.Value)
		if err != nil {
			http.Error(w, "invalid demo session", 401)
			return "", false
		}
		return string(username), true
	}
	mux.HandleFunc("GET /profile", func(w http.ResponseWriter, r *http.Request) {
		username, ok := authenticated(w, r)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"authenticated": true, "user": map[string]string{"id": username}})
	})
	mux.HandleFunc("POST /checkout", func(w http.ResponseWriter, r *http.Request) {
		username, ok := authenticated(w, r)
		if !ok {
			return
		}
		var input struct {
			Item string `json:"item"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil || strings.TrimSpace(input.Item) == "" {
			http.Error(w, "item required", 400)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "work capacity exceeded", 503)
			return
		}
		timer := time.NewTimer(*delay)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "order_id": "demo-" + username, "item": input.Item})
	})
	mux.HandleFunc("/work", func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "work capacity exceeded", http.StatusServiceUnavailable)
			return
		}
		timer := time.NewTimer(*delay)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"ok":true}`)
		}
	})
	server := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()
	log.Printf("demo target: http://%s/work (capacity %d, work %s)", *addr, *capacity, *delay)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
