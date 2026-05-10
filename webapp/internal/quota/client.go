package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ScopeOCR    = "ocr_pages"
	ScopeAI     = "ai_requests"
	ScopeExport = "exports"

	dailyWindow = 24 * time.Hour
)

type Status struct {
	Scope        string    `json:"scope"`
	Window       string    `json:"window"`
	Limit        int       `json:"limit"`
	CurrentCount int       `json:"current_count"`
	Remaining    int       `json:"remaining"`
	IsExceeded   bool      `json:"is_exceeded"`
	ResetAt      time.Time `json:"reset_at"`
}

type IncrementResponse struct {
	Status   Status `json:"status"`
	Accepted bool   `json:"accepted"`
}

type Client struct {
	rdb    *redis.Client
	limits map[string]int // scope -> daily limit (0 = unlimited)
}

func New(redisURL string, limits map[string]int) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("quota: %w", err)
	}
	return &Client{rdb: redis.NewClient(opt), limits: limits}, nil
}

func (c *Client) Increment(ctx context.Context, userID, scope string) (*IncrementResponse, error) {
	now := time.Now().UTC()
	windowStart := now.Truncate(dailyWindow)
	key := fmt.Sprintf("quota:%s:%s:%d", userID, scope, windowStart.Unix())

	pipe := c.rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireAt(ctx, key, windowStart.Add(dailyWindow))
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	count := int(incr.Val())
	limit := c.limits[scope]
	remaining := -1
	exceeded := false
	if limit > 0 {
		remaining = limit - count
		remaining = max(remaining, 0)
		exceeded = count > limit
	}

	status := Status{
		Scope:        scope,
		Window:       "24h",
		Limit:        limit,
		CurrentCount: count,
		Remaining:    remaining,
		IsExceeded:   exceeded,
		ResetAt:      windowStart.Add(dailyWindow),
	}
	return &IncrementResponse{Status: status, Accepted: !exceeded}, nil
}
