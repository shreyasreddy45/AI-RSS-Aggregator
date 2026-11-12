package scraper

import (
	"context"
	"log"
	"log/slog"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	queue "github.com/shreyasreddy45/RSS-server/internal/queue"
)

// StartScraping runs the background worker loop that periodically fetches
// feeds due for scraping and enqueues each one for the production job queue.
func StartScraping(
	ctx context.Context,
	db *database.Queries,
	redisClient *redis.Client,
	concurrency int,
	timeout time.Duration,
	logger *slog.Logger,
	jobQueue *queue.Queue,
) {
	logger.Info(
		"Background scraper started",
		"workers", concurrency,
		"interval", timeout,
	)
	ticker := time.NewTicker(timeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping scraper...")
			return
		case <-ticker.C:
			feeds, err := db.GetNextFeedsToFetch(ctx, int32(concurrency))
			if err != nil {
				logger.Error(
					"Failed to fetch feeds",
					"error", err,
				)
				continue
			}

			for _, feed := range feeds {
				if err := jobQueue.Enqueue(queue.Job{
					Type:       queue.JobTypeFeedScrape,
					Payload:    feed,
					MaxRetries: 3,
				}); err != nil {
					logger.Error(
						"Failed to enqueue feed scrape job",
						"feed_id", feed.ID,
						"error", err,
					)
				}
			}
		}
	}
}
