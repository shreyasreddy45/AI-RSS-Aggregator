package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobType identifies the type of work that should be executed by the queue.
type JobType string

const (
	JobTypeFeedScrape JobType = "feed_scrape"
	// JobTypeAIProcess asks the Python AI service to generate the summary and
	// embedding for a post. The payload is the post's uuid.UUID.
	JobTypeAIProcess JobType = "ai_process"
)

// Handler processes a queued job.
type Handler func(context.Context, Job) error

// Job is a single unit of work handled by the queue.
type Job struct {
	ID         string
	Type       JobType
	Payload    interface{}
	MaxRetries int
	Attempt    int
	EnqueuedAt time.Time
}

// Queue is a worker-driven job queue with retry and exponential backoff support.
type Queue struct {
	jobs     chan Job
	logger   *slog.Logger
	workers  int
	handlers map[JobType]Handler
	stop     chan struct{}
	mu       sync.RWMutex
	once     sync.Once
	wg       sync.WaitGroup
}

func New(logger *slog.Logger, workers int, handlers map[JobType]Handler) *Queue {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	if workers <= 0 {
		workers = 1
	}
	if handlers == nil {
		handlers = make(map[JobType]Handler)
	}

	return &Queue{
		jobs:     make(chan Job, 128),
		logger:   logger,
		workers:  workers,
		handlers: handlers,
		stop:     make(chan struct{}),
	}
}

func (q *Queue) Run(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}
	q.logger.Info("Job queue started", "workers", q.workers)
}

func (q *Queue) Stop() {
	q.once.Do(func() {
		close(q.stop)
	})
	q.wg.Wait()
}

func (q *Queue) Enqueue(job Job) error {
	q.mu.RLock()
	_, ok := q.handlers[job.Type]
	q.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unsupported job type: %s", job.Type)
	}
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now().UTC()
	}
	if job.MaxRetries < 0 {
		job.MaxRetries = 0
	}

	select {
	case <-q.stop:
		return errors.New("queue is stopped")
	case q.jobs <- job:
		return nil
	}
}

func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()
	for {
		select {
		case <-ctx.Done():
			q.logger.Info("Queue worker stopping", "worker_id", id)
			return
		case <-q.stop:
			q.logger.Info("Queue worker stopped", "worker_id", id)
			return
		case job, ok := <-q.jobs:
			if !ok {
				return
			}
			if err := q.process(ctx, job); err != nil {
				q.logger.Error(
					"Job failed",
					"job_id", job.ID,
					"job_type", job.Type,
					"attempt", job.Attempt,
					"max_retries", job.MaxRetries,
					"error", err,
				)
				if q.shouldRetry(job) {
					backoff := time.Duration(1<<min(job.Attempt, 5)) * time.Second
					q.logger.Warn("Retrying job with backoff", "job_id", job.ID, "delay", backoff)
					time.AfterFunc(backoff, func() {
						job.Attempt++
						if enqueueErr := q.Enqueue(job); enqueueErr != nil {
							q.logger.Error("Failed to re-enqueue job", "job_id", job.ID, "error", enqueueErr)
						}
					})
				}
			}
		}
	}
}

func (q *Queue) process(ctx context.Context, job Job) error {
	q.mu.RLock()
	handler, ok := q.handlers[job.Type]
	q.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unsupported job type: %s", job.Type)
	}
	if handler == nil {
		return fmt.Errorf("handler is nil for job type: %s", job.Type)
	}
	return handler(ctx, job)
}

func (q *Queue) shouldRetry(job Job) bool {
	return job.Attempt < job.MaxRetries
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
