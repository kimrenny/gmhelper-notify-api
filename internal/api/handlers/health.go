package handlers

import (
	"net/http"

	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type HealthHandler struct {
	readiness *health.ReadinessService
	logger    logger.Logger
}

func NewHealthHandler(readiness *health.ReadinessService, logger logger.Logger) *HealthHandler {
	return &HealthHandler{readiness: readiness, logger: logger}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := h.readiness.Check(r.Context()); err != nil {
		h.logger.Error("readiness probe failed", logger.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"unavailable"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ready"}`))
}
