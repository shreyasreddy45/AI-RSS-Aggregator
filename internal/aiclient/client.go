// Package aiclient is the Go side of the boundary with the Python AI service.
//
// All AI work (summaries, embeddings, RAG answers, the agent, semantic search)
// lives in ./ai_service. This package only knows how to talk to it over HTTP:
//
//   - it authenticates every call with a shared secret (X-Internal-Token),
//   - it tells the service which user the call is for (X-User-ID), after the Go
//     API has already validated that user's JWT,
//   - it never interprets AI responses; user-facing calls are passed through.
package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	headerToken  = "X-Internal-Token"
	headerUserID = "X-User-ID"

	// maxResponseBytes bounds how much of an upstream response is buffered.
	maxResponseBytes = 8 << 20
)

// ErrUnavailable is returned when the AI service cannot be reached.
var ErrUnavailable = errors.New("ai service unavailable")

// Client calls the Python AI service.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Response is a buffered upstream response.
type Response struct {
	Status      int
	ContentType string
	Body        []byte
}

// New builds a client. Per-request deadlines come from the caller's context, so
// the underlying http.Client has no global timeout of its own.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        32,
				MaxIdleConnsPerHost: 32,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Do performs a request against the AI service on behalf of userID. userID may
// be uuid.Nil for system calls (queue jobs) that are not tied to a user.
func (c *Client) Do(ctx context.Context, method, path string, userID uuid.UUID, query url.Values, body []byte) (*Response, error) {
	if c == nil {
		return nil, ErrUnavailable
	}

	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("build ai request: %w", err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set(headerToken, c.token)
	}
	if userID != uuid.Nil {
		req.Header.Set(headerUserID, userID.String())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read ai response: %w", err)
	}

	return &Response{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        data,
	}, nil
}

// ProcessPost asks the AI service to generate whatever is still missing for a
// post (summary and embedding). It is idempotent, which makes it safe to retry.
func (c *Client) ProcessPost(ctx context.Context, postID uuid.UUID) error {
	resp, err := c.Do(ctx, http.MethodPost, "/internal/posts/"+postID.String()+"/process", uuid.Nil, nil, nil)
	if err != nil {
		return err
	}
	if resp.Status != http.StatusOK {
		return fmt.Errorf("ai service returned %d: %s", resp.Status, truncate(string(resp.Body), 300))
	}
	return nil
}

// PendingPosts returns IDs of posts that still lack a summary or an embedding.
func (c *Client) PendingPosts(ctx context.Context, limit int) ([]uuid.UUID, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	resp, err := c.Do(ctx, http.MethodGet, "/internal/posts/pending", uuid.Nil, q, nil)
	if err != nil {
		return nil, err
	}
	if resp.Status != http.StatusOK {
		return nil, fmt.Errorf("ai service returned %d: %s", resp.Status, truncate(string(resp.Body), 300))
	}

	var payload struct {
		PostIDs []uuid.UUID `json:"post_ids"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, fmt.Errorf("decode pending posts: %w", err)
	}
	return payload.PostIDs, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
