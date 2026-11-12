package main

//	@title						RSS Server API
//	@version					1.0
//	@description				AI-powered RSS aggregation API (Go) backed by a Python AI service. JWT bearer authentication.
//	@host						localhost:8080
//	@BasePath					/v1
//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						Authorization
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@security					BearerAuth
//	@security					ApiKeyAuth

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/backfill"
	"github.com/shreyasreddy45/RSS-server/internal/config"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	"github.com/shreyasreddy45/RSS-server/internal/logger"
	queue "github.com/shreyasreddy45/RSS-server/internal/queue"
	"github.com/shreyasreddy45/RSS-server/internal/router"
	"github.com/shreyasreddy45/RSS-server/internal/scraper"
)

func main() {

	appLogger := logger.New()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	a := app.New(cfg, appLogger)
	defer a.SQLDB.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var jobQueue *queue.Queue
	jobQueue = queue.New(a.Logger, a.Config.QueueWorkers, map[queue.JobType]queue.Handler{
		queue.JobTypeFeedScrape: func(ctx context.Context, job queue.Job) error {
			feed, ok := job.Payload.(database.Feed)
			if !ok {
				return fmt.Errorf("unexpected payload type %T for feed scrape job", job.Payload)
			}
			return scraper.ProcessFeedJob(ctx, a.DB, a.Redis, feed, a.Logger, jobQueue)
		},
		queue.JobTypeAIProcess: func(ctx context.Context, job queue.Job) error {
			postID, ok := job.Payload.(uuid.UUID)
			if !ok {
				return fmt.Errorf("unexpected payload type %T for ai process job", job.Payload)
			}
			if a.AI == nil {
				a.Logger.Warn("Skipping AI processing because the AI service is not configured", "post_id", postID)
				return nil
			}
			// The AI service is idempotent: it only generates what is missing,
			// so a failed job can safely be retried by the queue.
			callCtx, callCancel := context.WithTimeout(ctx, a.Config.AIRequestTimeout)
			defer callCancel()
			return a.AI.ProcessPost(callCtx, postID)
		},
	})
	jobQueue.Run(ctx)

	// Backfill in the background (now and every 15 minutes) so a large backlog
	// never delays startup and posts missed during an AI outage are retried.
	go backfill.RunPeriodic(ctx, a.AI, jobQueue, a.Logger)

	go scraper.StartScraping(
		ctx,
		a.DB,
		a.Redis,
		a.Config.MaxConcurrency,
		a.Config.ScrapeInterval,
		a.Logger,
		jobQueue,
	)

	r := router.New(a)

	a.Run(ctx, cancel, r)
}
