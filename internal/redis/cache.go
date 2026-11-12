package redis

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	redislib "github.com/redis/go-redis/v9"
)

func ShouldCache(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL == nil {
		return false
	}

	return r.URL.Path == "/v1/feeds" || strings.HasPrefix(r.URL.Path, "/v1/posts")
}

func CacheKey(r *http.Request) string {
	keyParts := []string{"rss-cache", r.Method, r.URL.Path}
	if userID, ok := r.Context().Value("userID").(string); ok && userID != "" {
		keyParts = append(keyParts, "user", userID)
	}
	if r.URL.RawQuery != "" {
		keyParts = append(keyParts, "query", r.URL.RawQuery)
	}
	return strings.Join(keyParts, ":")
}

func InvalidatePattern(ctx context.Context, client *redislib.Client, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return nil
}

func InvalidateFeedAndPostCache(ctx context.Context, client *redislib.Client) error {
	patterns := []string{
		"rss-cache:GET:/v1/feeds*",
		"rss-cache:GET:/v1/posts*",
	}
	for _, pattern := range patterns {
		if err := InvalidatePattern(ctx, client, pattern); err != nil {
			return fmt.Errorf("invalidate pattern %s: %w", pattern, err)
		}
	}
	return nil
}
