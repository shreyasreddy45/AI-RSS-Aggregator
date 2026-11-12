package scraper

import (
	"context"
	"database/sql"
	"io"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	queue "github.com/shreyasreddy45/RSS-server/internal/queue"
	cacheutil "github.com/shreyasreddy45/RSS-server/internal/redis"
	"github.com/shreyasreddy45/RSS-server/internal/rss"
)

func enqueueAIJob(ctx context.Context, jobQueue *queue.Queue, post database.Post, logger *slog.Logger) {
	_ = ctx
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if jobQueue == nil {
		logger.Warn("Skipping AI processing because the queue is not configured", "post_id", post.ID, "url", post.Url)
		return
	}
	if err := jobQueue.Enqueue(queue.Job{
		Type:       queue.JobTypeAIProcess,
		Payload:    post.ID,
		MaxRetries: 3,
	}); err != nil {
		logger.Error("Failed to queue AI processing job", "post_id", post.ID, "url", post.Url, "error", err)
		return
	}
	logger.Info("AI processing job queued", "post_id", post.ID, "url", post.Url)
}

func scrapeFeed(ctx context.Context, db *database.Queries, redisClient *redis.Client, feed database.Feed, logger *slog.Logger, jobQueue *queue.Queue) {
	if ctx == nil {
		ctx = context.Background()
	}

	_, err := db.MarkFeedasFetched(ctx, feed.ID)
	if err != nil {
		log.Printf("Error marking feed as fetched: %v", err)
		return
	}

	feedData, err := rss.UrlToFeed(feed.Url)
	if err != nil {
		log.Printf("Error fetching feed data for feed %s: %v", feed.ID, err)
		return
	}

	for _, item := range feedData.Channel.Item {

		description := sql.NullString{}
		if item.Description != "" {
			description = sql.NullString{
				String: item.Description,
				Valid:  true,
			}
		}

		pubAt, ok := rss.ParseFeedTime(item.PubDate)
		if !ok {
			// Never store the zero time: it renders as "Jan 1, 1" and sorts as ancient.
			log.Printf("couldn't parse date %q, using fetch time instead", item.PubDate)
			pubAt = time.Now().UTC()
		}

		createdPost, err := db.CreatePost(ctx, database.CreatePostParams{
			ID:          uuid.New(),
			Title:       item.Title,
			Description: description,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			PublishedAt: pubAt,
			Url:         item.Link,
			FeedID:      feed.ID,
		})

		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				continue
			}
			logger.Error(
				"Failed to create post",
				"title", item.Title,
				"url", item.Link,
				"error", err,
			)
			continue
		}

		enqueueAIJob(ctx, jobQueue, createdPost, logger)
	}

	if redisClient != nil {
		if err := cacheutil.InvalidateFeedAndPostCache(ctx, redisClient); err != nil {
			logger.Error("Failed to invalidate response cache", "error", err)
		}
	}

	log.Printf("Successfully fetched feed: %s", feed.Name)
}

// ProcessFeedJob executes a feed scraping task from the production queue.
func ProcessFeedJob(ctx context.Context, db *database.Queries, redisClient *redis.Client, feed database.Feed, logger *slog.Logger, jobQueue *queue.Queue) error {
	scrapeFeed(ctx, db, redisClient, feed, logger, jobQueue)
	return nil
}
