package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/crinne/swarm-drone-controller/internal/spawn"
)

type Spawner interface {
	Spawn(ctx context.Context, password string) (int, error)
}

func NewHandler(spawner Spawner, allowedOrigin string) http.Handler {
	mux := http.NewServeMux()

	addCORS := func(w http.ResponseWriter) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Spawn-Password")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Vary", "Origin")
	}

	writeJSON := func(w http.ResponseWriter, status int, value any) {
		body, err := json.Marshal(value)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("/spawn", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		id, err := spawner.Spawn(r.Context(), r.Header.Get("X-Spawn-Password"))
		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, map[string]int{"id": id})
		case errors.Is(err, spawn.ErrUnauthorized):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		case errors.Is(err, spawn.ErrConflict):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "spawn conflict"})
		case errors.Is(err, spawn.ErrLimitReached):
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "drone limit reached"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
