package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type dummyPinger struct {
	err error
}

func (d *dummyPinger) Ping(ctx context.Context) error {
	return d.err
}

func TestRouter_HealthAndReady(t *testing.T) {
	log, _ := logger.NewLogger("info")
	pinger := &dummyPinger{err: nil}
	readiness := health.NewReadinessService(pinger)
	healthHandler := handlers.NewHealthHandler(readiness, log)
	router := NewRouter(healthHandler, nil)

	// 1. GET /health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	router.ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Errorf("expected status 200 on /health, got %d", recHealth.Code)
	}

	// 2. GET /ready (healthy)
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	recReady := httptest.NewRecorder()
	router.ServeHTTP(recReady, reqReady)

	if recReady.Code != http.StatusOK {
		t.Errorf("expected status 200 on /ready, got %d", recReady.Code)
	}

	// 3. GET /ready (unhealthy)
	pinger.err = errors.New("db down")
	recReadyDown := httptest.NewRecorder()
	router.ServeHTTP(recReadyDown, reqReady)

	if recReadyDown.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 on /ready when db down, got %d", recReadyDown.Code)
	}
}

func TestRouter_NotFoundJSON(t *testing.T) {
	log, _ := logger.NewLogger("info")
	pinger := &dummyPinger{err: nil}
	readiness := health.NewReadinessService(pinger)
	healthHandler := handlers.NewHealthHandler(readiness, log)
	router := NewRouter(healthHandler, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown-endpoint", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected valid JSON error response, got error: %v (body: %s)", err, rec.Body.String())
	}

	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected error code NOT_FOUND, got %s", errResp.Error.Code)
	}
}
