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
	"sync"
	"syscall"
	"time"

	"github.com/gmhelper/notify-api/internal/api"
	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/campaign"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/app/template"
	"github.com/gmhelper/notify-api/internal/app/user"
	"github.com/gmhelper/notify-api/internal/config"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/postgres"
	"github.com/gmhelper/notify-api/internal/infra/smtp"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
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

	campaignRepo := postgres.NewNotificationCampaignRepository(db.DB())
	campaignService := campaign.NewService(campaignRepo)
	campaignHandler := handlers.NewCampaignHandler(campaignService, log)
	recipientRepo := postgres.NewCampaignRecipientRepository(db.DB())

	directRepo := postgres.NewDirectNotificationRepository(db.DB())
	attemptRepo := postgres.NewDeliveryAttemptRepository(db.DB())
	smtpSender := smtp.NewClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)

	serviceTokenProvider, err := auth.NewServiceTokenProvider(auth.ServiceTokenProviderConfig{
		Secret:   cfg.ServiceAuthSecret,
		Issuer:   cfg.AuthIssuer,
		Audience: cfg.ServiceAuthAudience,
	})
	if err != nil {
		log.Fatal("failed to initialize service token provider", zapError(err))
	}

	var userHTTPClient userclient.Client
	var userService *user.Service
	if cfg.GMHelperAPIBaseURL != "" {
		var clientErr error
		userHTTPClient, clientErr = userclient.NewClient(cfg.GMHelperAPIBaseURL, nil, serviceTokenProvider)
		if clientErr != nil {
			log.Fatal("failed to initialize gmhelper-api user client", zapError(clientErr))
		}
		userService, err = user.NewService(userHTTPClient)
		if err != nil {
			log.Fatal("failed to initialize user service", zapError(err))
		}
		log.Info("gmhelper-api user resolution service initialized", zapString("baseURL", cfg.GMHelperAPIBaseURL))
	} else {
		log.Info("gmhelper-api base URL not configured; user resolution service is disabled")
	}

	directService := direct.NewService(templateRepo, directRepo, userService)
	deliveryService := direct.NewDeliveryServiceWithMaxAttempts(directRepo, attemptRepo, templateRepo, smtpSender, cfg.WorkerMaxAttempts)
	directHandler := handlers.NewDirectNotificationHandler(directService, deliveryService, log)

	var userHandler *handlers.UserHandler
	if userService != nil {
		userHandler = handlers.NewUserHandler(userService, log)
	}

	jwtVerifier, err := auth.NewJWTVerifier(cfg.AuthSecret, cfg.AuthIssuer, cfg.AuthAudience)
	if err != nil {
		log.Fatal("failed to initialize jwt verifier", zapError(err))
	}
	authMiddleware := middleware.AdminAuth(jwtVerifier, log)

	router := api.NewRouter(healthHandler, templateHandler, campaignHandler, directHandler, userHandler, authMiddleware)
	handler := middleware.Chain(router,
		middleware.RequestID(),
		middleware.Logging(log),
		middleware.Recovery(log),
		middleware.CORS(cfg.AllowedCORSOrigins),
	)

	// Direct notification background delivery worker
	var workerWg sync.WaitGroup
	if cfg.WorkerEnabled {
		worker := direct.NewWorker(directRepo, deliveryService, cfg.WorkerInterval, cfg.WorkerStaleTimeout, cfg.WorkerMaxAttempts, log)
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			worker.Start(ctx)
		}()
	} else {
		log.Info("direct notification background worker is disabled")
	}

	// Campaign background scheduler
	if cfg.SchedulerEnabled {
		var populator campaign.AudiencePopulator
		if userHTTPClient != nil {
			populator = campaign.NewAudiencePopulator(userHTTPClient, recipientRepo, campaignRepo, log)
		}
		scheduler := campaign.NewScheduler(campaignRepo, populator, cfg.SchedulerInterval, cfg.SchedulerBatchSize, log)
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			scheduler.Start(ctx)
		}()
	} else {
		log.Info("campaign background scheduler is disabled")
	}

	// Campaign background delivery worker
	if cfg.CampaignWorkerEnabled {
		campaignDeliveryService := campaign.NewDeliveryService(campaignRepo, recipientRepo, templateRepo, attemptRepo, smtpSender, userService)
		campaignWorker := campaign.NewWorker(recipientRepo, campaignRepo, campaignDeliveryService, cfg.CampaignWorkerInterval, cfg.CampaignWorkerStaleTimeout, cfg.CampaignWorkerBatchSize, log)
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			campaignWorker.Start(ctx)
		}()
	} else {
		log.Info("campaign delivery background worker is disabled")
	}

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

	// Cancel root context to signal background workers to stop
	cancel()
	workerWg.Wait()

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
