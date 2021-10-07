package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sethvargo/github-workflow-job-to-pubsub/internal/pubsub"
)

type WorkflowJobEvent struct {
	Action string `json:"action"`

	WorkflowJob struct {
		RunID  string `json:"run_id"`
		RunURL string `json:"run_url"`
	}

	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (s *Server) handleWebhook() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		defer r.Body.Close()

		if !isWorkflowJobEvent(r) {
			respondJSON(w, &jsonResponse{
				Error: "invalid event type",
			}, http.StatusBadRequest)
			return
		}

		limitedBody := io.LimitReader(r.Body, 12*1024*1024) // 12 MiB
		body, err := io.ReadAll(limitedBody)
		if err != nil {
			logger.Error("failed to read body", "error", err)
			respondJSON(w, &jsonResponse{
				Error: fmt.Sprintf("failed to read body: %s", err),
			}, http.StatusBadRequest)
			return
		}

		givenSignature := r.Header.Get(GitHubSignatureHeaderName)
		if !isValidSignature(givenSignature, body) {
			respondJSON(w, &jsonResponse{
				Error: "invalid signature",
			}, http.StatusBadRequest)
			return
		}

		// If we got this far, it's safe to try and decode the payload.
		var event WorkflowJobEvent
		if err := json.Unmarshal(body, &event); err != nil {
			logger.Error("failed to unmarshal event", "error", err)
			respondJSON(w, &jsonResponse{
				Error: fmt.Sprintf("failed to unmarshal json: %s", err),
			}, http.StatusInternalServerError)
			return
		}

		switch event.Action {
		case StatusQueued:
			// Increase the size of the pool by adding a message to the queue.
			if err := s.pubsubClient.PublishOne(ctx, PubSubTopicName, &pubsub.Message{
				Attributes: map[string]string{
					"run_id":  event.WorkflowJob.RunID,
					"run_url": event.WorkflowJob.RunURL,
				},
			}); err != nil {
				logger.Error("failed to publish", "error", err)
				respondJSON(w, &jsonResponse{
					Error: fmt.Sprintf("failed to publish message: %s", err),
				}, http.StatusInternalServerError)
			}

		case StatusInProgress:
			// Do nothing, we already queued a worker for the queued job.
			respondJSON(w, &jsonResponse{
				Message: "ok",
			}, http.StatusOK)

		case StatusCompleted:
			// Remove an item from the pool.
			ctx, done := context.WithTimeout(context.Background(), 7*time.Second)
			defer done()

			if _, err := s.pubsubClient.PullAndAck(ctx, PubSubSubscriptionName); err != nil {
				logger.Error("failed to pull and ack", "error", err)
				respondJSON(w, &jsonResponse{
					Error: fmt.Sprintf("failed to pull and ack: %s", err),
				}, http.StatusInternalServerError)
			}
		default:
			logger.Error("unknown event action", "action", event.Action)
		}
	})
}

// isWorkflowJobEvent verifies the request is of type "workflow_job", which
// indicates it's safe to attempt to unmarshal into the WorkflowJob struct.
func isWorkflowJobEvent(r *http.Request) bool {
	return r.Header.Get(GitHubEventHeaderName) == GitHubEventWorkflowJob
}
