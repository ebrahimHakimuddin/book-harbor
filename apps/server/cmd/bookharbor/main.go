package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/audit"
	"github.com/bookharbor/bookharbor/apps/server/internal/config"
	"github.com/bookharbor/bookharbor/apps/server/internal/database"
	"github.com/bookharbor/bookharbor/apps/server/internal/export"
	"github.com/bookharbor/bookharbor/apps/server/internal/httpapi"
	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"github.com/bookharbor/bookharbor/apps/server/internal/mail"
	"github.com/bookharbor/bookharbor/apps/server/internal/metadata"
	"github.com/bookharbor/bookharbor/apps/server/internal/reading"
	"github.com/bookharbor/bookharbor/apps/server/internal/social"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		runRestore(os.Args[2:])
		return
	}

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
	), mail.NewZeptoMail(
		os.Getenv("BOOKHARBOR_ZEPTOMAIL_TOKEN"),
		os.Getenv("BOOKHARBOR_ZEPTOMAIL_FROM_EMAIL"),
		os.Getenv("BOOKHARBOR_ZEPTOMAIL_FROM_NAME"),
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

// runRestore implements `bookharbor restore <archive.zip> [--force]`: an offline
// operation, run instead of starting the server, that replaces BOOKHARBOR_DATA_DIR's
// database and book/cover files with the contents of a backup produced by the
// admin "Export everything" feature. It must not run against a data directory a
// server is currently using.
func runRestore(args []string) {
	set := flag.NewFlagSet("restore", flag.ExitOnError)
	force := set.Bool("force", false, "overwrite an existing database in the data directory")
	set.Parse(args)
	if set.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: bookharbor restore [--force] <archive.zip>")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	if err := export.Restore(set.Arg(0), cfg.DataDir, *force); err != nil {
		if errors.Is(err, export.ErrDataDirNotEmpty) {
			fmt.Fprintf(os.Stderr, "%v\nStop the server first; this replaces its database and files. Pass --force to proceed.\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
		}
		os.Exit(1)
	}
	fmt.Printf("Restored %s into %s. Start the server normally to use it.\n", set.Arg(0), cfg.DataDir)
}
