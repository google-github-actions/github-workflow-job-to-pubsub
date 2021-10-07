package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sethvargo/github-workflow-job-to-pubsub/internal/logging"
	"github.com/sethvargo/github-workflow-job-to-pubsub/internal/pubsub"
)

const (
	// GitHubSignatureHeaderName is the header to search for the GitHub webhook
	// payload signature. GitHubSignaturePrefix is the message prefix on the
	// signature.
	GitHubSignatureHeaderName = "x-hub-signature-256"
	GitHubSignaturePrefix     = "sha256="

	// GitHubEventHeaderName is the header for the event name.
	// GitHubEventWorkflowJob is the event name for job queueing.
	GitHubEventHeaderName  = "x-github-event"
	GitHubEventWorkflowJob = "workflow_job"

	// Status event types.
	StatusQueued     = "queued"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
)

var (
	// GitHubWebhookSecret is the secret key to use for authenticating the
	// webhook's HMAC. This should be injected via Secret Manager or a similar
	// process.
	GitHubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")

	// PubSubTopicName is the name of the topic on which to publish.
	// PubSubSubscriptionName is the name of the subscription on which to pull.
	// These should be the full topic and subscription including the project (e.g.
	// "projects/p/topics/t").
	PubSubTopicName        = os.Getenv("PUBSUB_TOPIC_NAME")
	PubSubSubscriptionName = os.Getenv("PUBSUB_SUBSCRIPTION_NAME")

	// logger is the logging system.
	logger = logging.NewLogger(os.Stdout, os.Stderr)
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer done()

	err := realMain(ctx)
	done()

	if err != nil {
		logger.Fatal("application failure", "error", err)
	}
	logger.Info("shutting down")
}

func realMain(ctx context.Context) error {
	pubsubClient, err := pubsub.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create pubsub client: %w", err)
	}

	s := &Server{
		pubsubClient: pubsubClient,
	}

	mux := http.NewServeMux()
	mux.Handle("/", s.handleWebhook())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("failed to listen and serve", "error", err)
		}
	}()
	<-ctx.Done()

	shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}
