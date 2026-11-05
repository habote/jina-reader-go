package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPingHandler_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	PingHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	expectedContentType := "application/json; charset=utf-8"
	if contentType != expectedContentType {
		t.Errorf("expected Content-Type %q, got %q", expectedContentType, contentType)
	}

	var resp PingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status %q, got %q", "ok", resp.Status)
	}
	if resp.Version != "0.1.0" {
		t.Errorf("expected version %q, got %q", "0.1.0", resp.Version)
	}
	if resp.Uptime != "running" {
		t.Errorf("expected uptime %q, got %q", "running", resp.Uptime)
	}
	if resp.GitCommit != GitCommit {
		t.Errorf("expected git_commit %q, got %q", GitCommit, resp.GitCommit)
	}
	if resp.BuildTime != BuildTime {
		t.Errorf("expected build_time %q, got %q", BuildTime, resp.BuildTime)
	}
}

func TestPingHandler_CustomBuildInfo(t *testing.T) {
	origCommit := GitCommit
	origBuildTime := BuildTime
	t.Cleanup(func() {
		GitCommit = origCommit
		BuildTime = origBuildTime
	})

	GitCommit = "abc1234"
	BuildTime = "2026-11-05T00:00:00Z"

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	PingHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	var resp PingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if resp.GitCommit != "abc1234" {
		t.Errorf("expected git_commit %q, got %q", "abc1234", resp.GitCommit)
	}
	if resp.BuildTime != "2026-11-05T00:00:00Z" {
		t.Errorf("expected build_time %q, got %q", "2026-11-05T00:00:00Z", resp.BuildTime)
	}
}

func TestPingHandler_MethodNotAllowed(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/ping", nil)
			rec := httptest.NewRecorder()

			PingHandler(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d for %s, got %d", http.StatusMethodNotAllowed, method, rec.Code)
			}
		})
	}
}

func TestRouter_Ping(t *testing.T) {
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	var resp PingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if resp.Status != "ok" || resp.Version != "0.1.0" || resp.Uptime != "running" || resp.GitCommit != GitCommit || resp.BuildTime != BuildTime {
		t.Errorf("unexpected response: %+v", resp)
	}
}
