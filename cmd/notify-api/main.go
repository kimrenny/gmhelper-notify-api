package main

import (
	"context"
	"fmt"
	"net/http"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/gmhelper/notify-api/internal/api"
	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/config"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/postgres"
	"github.com/gmhelper/notify-api/internal/infra/smtp"
	"github.com/gmhelper/notify-api/internal/api/handlers"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.NewLogger(cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	db, err := postgres.NewPostgresDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("failed to connect to database", zapError(err))
	}
	defer func() {
		if err := db.Close(ctx); err != nil {
			log.Warn("failed to close database connection", zapError(err))
		}
	}()

	_ = smtp.NewSMTPClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
	readinessService := health.NewReadinessService(db)
	healthHandler := handlers.NewHealthHandler(readinessService, log)

	router := api.NewRouter(healthHandler)
	handler := middleware.Chain(router,
		middleware.RequestID(),
		middleware.Logging(log),
		middleware.Recovery(log),
		middleware.CORS(cfg.AllowedCORSOrigins),
	)

	server := &http.Server{
		Addr:         net.JoinHostPort(cfg.HTTPHost, strconv.Itoa(cfg.HTTPPort)),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Info("notify API starting", zapString("addr", server.Addr), zapString("env", cfg.Env))

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server failed", zapError(err))
	}
}

func zapString(key, value string) logger.Field {
	return logger.String(key, value)
}

func zapError(err error) logger.Field {
	return logger.Error(err)
}
