package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestFeedSubscriptionUpserts(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL subscription integration tests")
	}

	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	schema := "feed_subscription_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()

	statements := []string{
		"CREATE TABLE " + schema + ".users (id UUID PRIMARY KEY)",
		"CREATE TABLE " + schema + ".feeds (" +
			"id UUID PRIMARY KEY, name VARCHAR(100) NOT NULL, created_at TIMESTAMP NOT NULL, " +
			"updated_at TIMESTAMP NOT NULL, url TEXT UNIQUE NOT NULL, " +
			"user_id UUID NOT NULL REFERENCES " + schema + ".users(id), last_fetched_at TIMESTAMP)",
		"CREATE TABLE " + schema + ".feed_follows (" +
			"id UUID PRIMARY KEY, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, " +
			"user_id UUID NOT NULL REFERENCES " + schema + ".users(id), " +
			"feed_id UUID NOT NULL REFERENCES " + schema + ".feeds(id), UNIQUE(user_id, feed_id))",
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("create test tables: %v", err)
		}
	}

	users := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	for _, userID := range users {
		if _, err := db.ExecContext(ctx, "INSERT INTO "+schema+".users (id) VALUES ($1)", userID); err != nil {
			t.Fatalf("insert test user: %v", err)
		}
	}

	subscribe := func(userID uuid.UUID, feedURL string) (Feed, FeedFollow, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return Feed{}, FeedFollow{}, err
		}
		defer tx.Rollback()

		if _, err := tx.ExecContext(ctx, "SET LOCAL search_path TO "+schema); err != nil {
			return Feed{}, FeedFollow{}, err
		}
		q := New(tx)
		now := time.Now()
		feed, err := q.CreateFeed(ctx, CreateFeedParams{
			ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
			Name: "Test feed", Url: feedURL, UserID: userID,
		})
		if err != nil {
			return Feed{}, FeedFollow{}, err
		}
		follow, err := q.CreateFeedFollow(ctx, CreateFeedFollowParams{
			ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
			UserID: userID, FeedID: feed.ID,
		})
		if err != nil {
			return Feed{}, FeedFollow{}, err
		}
		if err := tx.Commit(); err != nil {
			return Feed{}, FeedFollow{}, err
		}
		return feed, follow, nil
	}

	firstFeed, firstFollow, err := subscribe(users[0], "https://example.com/news.rss")
	if err != nil {
		t.Fatalf("first user subscription: %v", err)
	}
	secondFeed, secondFollow, err := subscribe(users[1], "https://example.com/news.rss")
	if err != nil {
		t.Fatalf("second user subscription: %v", err)
	}
	if firstFeed.ID != secondFeed.ID {
		t.Fatalf("users got different feed IDs: %s and %s", firstFeed.ID, secondFeed.ID)
	}
	if firstFollow.UserID != users[0] || secondFollow.UserID != users[1] ||
		firstFollow.FeedID != firstFeed.ID || secondFollow.FeedID != firstFeed.ID {
		t.Fatal("users are not both associated with the shared feed")
	}
	if firstFollow.ID == secondFollow.ID {
		t.Fatal("different users unexpectedly share a feed-follow ID")
	}

	repeatedFeed, repeatedFollow, err := subscribe(users[0], "https://example.com/news.rss")
	if err != nil {
		t.Fatalf("repeated user subscription: %v", err)
	}
	if repeatedFeed.ID != firstFeed.ID || repeatedFollow.ID != firstFollow.ID {
		t.Fatal("repeated subscription did not return the existing feed and follow")
	}
	assertCount(t, ctx, db, schema, "feeds", "url", "https://example.com/news.rss", 1)
	assertCount(t, ctx, db, schema, "feed_follows", "feed_id", firstFeed.ID, 2)

	type result struct {
		feed   Feed
		follow FeedFollow
		err    error
	}
	start := make(chan struct{})
	ready := sync.WaitGroup{}
	ready.Add(2)
	results := make(chan result, 2)
	for _, userID := range users[2:] {
		userID := userID
		go func() {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				ready.Done()
				results <- result{err: err}
				return
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(ctx, "SET LOCAL search_path TO "+schema); err != nil {
				ready.Done()
				results <- result{err: err}
				return
			}
			ready.Done()
			<-start
			q := New(tx)
			now := time.Now()
			feed, err := q.CreateFeed(ctx, CreateFeedParams{
				ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
				Name: "Concurrent feed", Url: "https://example.com/concurrent.rss", UserID: userID,
			})
			if err == nil {
				var follow FeedFollow
				follow, err = q.CreateFeedFollow(ctx, CreateFeedFollowParams{
					ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
					UserID: userID, FeedID: feed.ID,
				})
				if err == nil {
					err = tx.Commit()
				}
				results <- result{feed: feed, follow: follow, err: err}
				return
			}
			results <- result{err: err}
		}()
	}
	ready.Wait()
	close(start)
	concurrent := []result{<-results, <-results}
	for _, result := range concurrent {
		if result.err != nil {
			t.Fatalf("concurrent subscription: %v", result.err)
		}
	}
	if concurrent[0].feed.ID != concurrent[1].feed.ID {
		t.Fatalf("concurrent users got different feed IDs: %s and %s", concurrent[0].feed.ID, concurrent[1].feed.ID)
	}
	for _, result := range concurrent {
		if result.follow.UserID == uuid.Nil || result.follow.FeedID != result.feed.ID {
			t.Fatal("concurrent subscription did not return its feed-follow relationship")
		}
	}
	assertCount(t, ctx, db, schema, "feeds", "url", "https://example.com/concurrent.rss", 1)
	assertCount(t, ctx, db, schema, "feed_follows", "feed_id", concurrent[0].feed.ID, 2)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin foreign-key test transaction: %v", err)
	}
	_, err = tx.ExecContext(ctx, "SET LOCAL search_path TO "+schema)
	if err != nil {
		tx.Rollback()
		t.Fatalf("set foreign-key test schema: %v", err)
	}
	_, err = New(tx).CreateFeedFollow(ctx, CreateFeedFollowParams{
		ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		UserID: uuid.New(), FeedID: firstFeed.ID,
	})
	tx.Rollback()
	if err == nil {
		t.Fatal("expected invalid user reference to fail")
	}
}

func assertCount(t *testing.T, ctx context.Context, db *sql.DB, schema, table, column string, value any, want int) {
	t.Helper()
	query := fmt.Sprintf("SELECT count(*) FROM %s.%s WHERE %s = $1", schema, table, column)
	var got int
	if err := db.QueryRowContext(ctx, query, value).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Errorf("%s count = %d, want %d", table, got, want)
	}
}
