package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/audit"
	"github.com/bookharbor/bookharbor/apps/server/internal/config"
	"github.com/bookharbor/bookharbor/apps/server/internal/database"
	"github.com/bookharbor/bookharbor/apps/server/internal/httpapi"
	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"github.com/bookharbor/bookharbor/apps/server/internal/metadata"
	"github.com/bookharbor/bookharbor/apps/server/internal/reading"
	"github.com/bookharbor/bookharbor/apps/server/internal/social"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		logger.Error("create data directory", "error", err)
		os.Exit(1)
	}
	db, err := database.Open(context.Background(), cfg.DataDir)
	if err != nil {
		logger.Error("open metadata database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	identityStore := identity.NewStore(db)
	libraryStore, err := library.NewStore(db, cfg.DataDir, cfg.MaxUploadBytes)
	if err != nil {
		logger.Error("initialize library storage", "error", err)
		os.Exit(1)
	}

	handler := httpapi.New(cfg, httpapi.BuildInfo{
		Version: version,
		Commit:  commit,
	}, identityStore, audit.NewStore(db), libraryStore, reading.NewStore(db), social.NewStore(db), metadata.NewHardcover(
		os.Getenv("BOOKHARBOR_HARDCOVER_TOKEN"),
		os.Getenv("BOOKHARBOR_HARDCOVER_ENDPOINT"),
		nil,
	), logger)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	shutdownSignal, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go func() {
		<-shutdownSignal.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Error("graceful shutdown", "error", err)
		}
	}()

	logger.Info("server starting", "address", cfg.Addr, "version", version)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}
