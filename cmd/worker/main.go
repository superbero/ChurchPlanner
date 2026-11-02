package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourchurch/churchplanner/internal/platform/config"
	"github.com/yourchurch/churchplanner/internal/platform/database"
	"github.com/yourchurch/churchplanner/internal/platform/mailer"
	"github.com/yourchurch/churchplanner/internal/scheduling"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connect", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	mail := mailer.New(cfg)
	repo := scheduling.NewAssignmentRepository(db)

	logger.Info("worker started")
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Run once immediately
	processNotifications(ctx, repo, mail, cfg.PublicBaseURL)

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			processNotifications(ctx, repo, mail, cfg.PublicBaseURL)
		case <-stop:
			logger.Info("worker stopping")
			return
		}
	}
}

func processNotifications(ctx context.Context, repo *scheduling.AssignmentRepository, mail *mailer.Mailer, baseURL string) {
	pending, err := repo.GetPendingNotifications(ctx)
	if err != nil {
		slog.Error("failed to fetch pending notifications", "err", err)
		return
	}

	if len(pending) == 0 {
		return
	}

	slog.Info("sending notifications", "count", len(pending))

	for _, p := range pending {
		url := baseURL + "/my-schedule"
		err := mail.SendAssignmentNotification(
			p.Email,
			p.Name,
			p.PositionName,
			p.PlanTitle,
			p.ServiceAt.Format("Mon 2 Jan 3:04 PM"),
			url,
		)
		if err != nil {
			slog.Error("failed to send email", "to", p.Email, "err", err)
			continue
		}

		if err := repo.MarkNotified(ctx, p.PlanPersonID); err != nil {
			slog.Error("failed to mark notified", "id", p.PlanPersonID, "err", err)
		}
	}
}
