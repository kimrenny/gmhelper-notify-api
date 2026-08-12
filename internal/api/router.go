package api

import (
	"net/http"

	"github.com/gmhelper/notify-api/internal/api/handlers"
)

type Router struct {
	handler http.Handler
}

func NewRouter(healthHandler *handlers.HealthHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", http.NotFoundHandler()))
	return mux
}
