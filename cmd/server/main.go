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

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/config"
	"github.com/yourchurch/churchplanner/internal/platform/database"
	"github.com/yourchurch/churchplanner/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connect", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		logger.Error("database migrate", "err", err)
		os.Exit(1)
	}

	sessions := auth.NewSessionManager(db)

	handler, err := web.NewRouter(web.Deps{
		DB:       db,
		Config:   cfg,
		Sessions: sessions,
	})
	if err != nil {
		logger.Error("router", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           sessions.LoadAndSave(handler),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("listening", "addr", srv.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("stopped")
}
