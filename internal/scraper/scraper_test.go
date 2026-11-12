package scraper

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	queue "github.com/shreyasreddy45/RSS-server/internal/queue"
)

func TestEnqueueAIJobQueuesPostIDAsync(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var seen uuid.UUID

	jobQueue := queue.New(logger, 1, map[queue.JobType]queue.Handler{
		queue.JobTypeAIProcess: func(ctx context.Context, job queue.Job) error {
			postID, ok := job.Payload.(uuid.UUID)
			if !ok {
				t.Fatalf("payload type mismatch: %T", job.Payload)
			}
			seen = postID
			return nil
		},
	})
	jobQueue.Run(context.Background())
	defer jobQueue.Stop()

	post := database.Post{ID: uuid.New(), Url: "https://example.com/article"}
	enqueueAIJob(context.Background(), jobQueue, post, logger)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if seen == post.ID {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected AI processing job to be queued and processed for post %s", post.ID)
}
