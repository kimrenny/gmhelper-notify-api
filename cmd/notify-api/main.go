package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gmhelper/notify-api/internal/api"
	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/app/template"
	"github.com/gmhelper/notify-api/internal/config"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/postgres"
	"github.com/gmhelper/notify-api/internal/infra/smtp"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		if err := db.Close(context.Background()); err != nil {
			log.Warn("failed to close database connection", zapError(err))
		}
	}()

	log.Info("running database migrations")
	if err := postgres.ApplyMigrations(ctx, db.DB()); err != nil {
		log.Fatal("database migration failed", zapError(err))
	}
	log.Info("database migrations applied successfully")

	readinessService := health.NewReadinessService(db)
	healthHandler := handlers.NewHealthHandler(readinessService, log)

	templateRepo := postgres.NewEmailTemplateRepository(db.DB())
	templateService := template.NewService(templateRepo)
	templateHandler := handlers.NewTemplateHandler(templateService, log)

	directRepo := postgres.NewDirectNotificationRepository(db.DB())
	attemptRepo := postgres.NewDeliveryAttemptRepository(db.DB())
	smtpSender := smtp.NewClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)

	directService := direct.NewService(templateRepo, directRepo)
	deliveryService := direct.NewDeliveryService(directRepo, attemptRepo, templateRepo, smtpSender)
	directHandler := handlers.NewDirectNotificationHandler(directService, deliveryService, log)

	router := api.NewRouter(healthHandler, templateHandler, directHandler)
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

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("notify API starting", zapString("addr", server.Addr), zapString("env", cfg.Env))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server failed", zapError(err))
		}
	}()

	sig := <-shutdownChan
	log.Info("shutdown signal received", zapString("signal", sig.String()))

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("server graceful shutdown failed", zapError(err))
	} else {
		log.Info("server stopped gracefully")
	}
}

func zapString(key, value string) logger.Field {
	return logger.String(key, value)
}

func zapError(err error) logger.Field {
	return logger.Error(err)
}
