package backfill

import (
	"context"
	"log/slog"
	"time"

	"github.com/shreyasreddy45/RSS-server/internal/aiclient"
	queue "github.com/shreyasreddy45/RSS-server/internal/queue"
)

// pendingBatch bounds how many posts a single backfill pass queues, so a large
// backlog cannot starve newly scraped posts.
const pendingBatch = 1000

// BackfillAI queues AI processing (summary + embedding) for posts that are
// still missing either one. The Python AI service decides which posts are
// pending, because it owns the AI tables.
func BackfillAI(
	ctx context.Context,
	ai *aiclient.Client,
	q *queue.Queue,
	logger *slog.Logger,
) error {
	if logger == nil {
		logger = slog.Default()
	}
	if ai == nil {
		logger.Warn("AI backfill skipped because the AI service is not configured")
		return nil
	}
	if q == nil {
		logger.Warn("AI backfill skipped because the queue is not configured")
		return nil
	}

	logger.Info("Backfilling AI summaries and embeddings...")

	postIDs, err := ai.PendingPosts(ctx, pendingBatch)
	if err != nil {
		logger.Error("AI backfill failed", "error", err)
		return err
	}
	if len(postIDs) == 0 {
		logger.Info("No posts require AI backfill.")
		return nil
	}

	queued := 0
	for _, postID := range postIDs {
		if err := q.Enqueue(queue.Job{
			Type:       queue.JobTypeAIProcess,
			Payload:    postID,
			MaxRetries: 3,
		}); err != nil {
			logger.Error("AI backfill enqueue failed", "post_id", postID, "error", err)
			continue
		}
		queued++
	}

	logger.Info("AI backfill completed.", "queued", queued)
	return nil
}

// periodicInterval is how often posts that still lack a summary or embedding
// (for example after a provider outage) are queued again.
const periodicInterval = 15 * time.Minute

// RunPeriodic runs BackfillAI immediately and then every periodicInterval until
// ctx is cancelled. Start it in its own goroutine.
func RunPeriodic(
	ctx context.Context,
	ai *aiclient.Client,
	q *queue.Queue,
	logger *slog.Logger,
) {
	if logger == nil {
		logger = slog.Default()
	}
	if ai == nil {
		return
	}

	run := func() {
		if err := BackfillAI(ctx, ai, q, logger); err != nil {
			logger.Error("AI backfill failed", "error", err)
		}
	}

	run()

	ticker := time.NewTicker(periodicInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
