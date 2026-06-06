package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crinne/swarm-drone-controller/internal/spawn"
)

type fakeSpawner struct {
	id       int
	err      error
	password string
}

func (s *fakeSpawner) Spawn(_ context.Context, password string) (int, error) {
	s.password = password
	return s.id, s.err
}

func TestSpawnReturnsCreatedID(t *testing.T) {
	spawner := &fakeSpawner{id: 4}
	server := httptest.NewServer(NewHandler(spawner, "https://swarmgcs.dev"))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/spawn", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-Spawn-Password", "secret")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /spawn: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	if spawner.password != "secret" {
		t.Fatalf("password = %q, want secret", spawner.password)
	}
	requireBody(t, res, `{"id":4}`)
}

func TestSpawnRequiresPasswordHeader(t *testing.T) {
	spawner := &fakeSpawner{err: spawn.ErrUnauthorized}
	handler := NewHandler(spawner, "https://swarmgcs.dev")

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/spawn", nil)
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}

func TestSpawnReturns429AtLimit(t *testing.T) {
	spawner := &fakeSpawner{err: spawn.ErrLimitReached}
	handler := NewHandler(spawner, "https://swarmgcs.dev")

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/spawn", nil)
	req.Header.Set("X-Spawn-Password", "secret")
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", res.Code)
	}
}

func TestSpawnReturns409OnConflict(t *testing.T) {
	spawner := &fakeSpawner{err: spawn.ErrConflict}
	handler := NewHandler(spawner, "https://swarmgcs.dev")

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/spawn", nil)
	req.Header.Set("X-Spawn-Password", "secret")
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.Code)
	}
}

func TestSpawnReturns500OnUnexpectedError(t *testing.T) {
	spawner := &fakeSpawner{err: errors.New("boom")}
	handler := NewHandler(spawner, "https://swarmgcs.dev")

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/spawn", nil)
	req.Header.Set("X-Spawn-Password", "secret")
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.Code)
	}
}

func TestHealthAndReady(t *testing.T) {
	handler := NewHandler(&fakeSpawner{}, "https://swarmgcs.dev")

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/health", body: `{"status":"ok"}`},
		{path: "/ready", body: `{"status":"ready"}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			handler.ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.Code)
			}
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != "https://swarmgcs.dev" {
				t.Fatalf("cors origin = %q, want https://swarmgcs.dev", got)
			}
			if res.Body.String() != tc.body {
				t.Fatalf("body = %q, want %q", res.Body.String(), tc.body)
			}
		})
	}
}

func TestCORSAllowsSwarmUI(t *testing.T) {
	handler := NewHandler(&fakeSpawner{}, "https://swarmgcs.dev, https://www.swarmgcs.dev")

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/spawn", nil)
	req.Header.Set("Origin", "https://www.swarmgcs.dev")
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "https://www.swarmgcs.dev" {
		t.Fatalf("cors origin = %q, want https://www.swarmgcs.dev", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, X-Spawn-Password" {
		t.Fatalf("cors headers = %q, want Content-Type, X-Spawn-Password", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Methods"); got != "POST, OPTIONS" {
		t.Fatalf("cors methods = %q, want POST, OPTIONS", got)
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	handler := NewHandler(&fakeSpawner{}, "https://swarmgcs.dev, https://www.swarmgcs.dev")

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/spawn", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("cors origin = %q, want empty", got)
	}
}

func TestOptionsWildcard(t *testing.T) {
	handler := NewHandler(&fakeSpawner{}, "https://swarmgcs.dev")

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/unmatched", nil)
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "https://swarmgcs.dev" {
		t.Fatalf("cors origin = %q, want https://swarmgcs.dev", got)
	}
}

func requireBody(t *testing.T, res *http.Response, want string) {
	t.Helper()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
