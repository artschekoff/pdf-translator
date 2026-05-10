package queue

import (
	"context"

	"github.com/hibiken/asynq"
)

type Client struct {
	c *asynq.Client
}

func NewClient(redisURL string) *Client {
	opt, _ := asynq.ParseRedisURI(redisURL)
	return &Client{c: asynq.NewClient(opt)}
}

func (c *Client) Close() { c.c.Close() }

func (c *Client) EnqueueOCR(ctx context.Context, p OcrPayload) error {
	b, err := Encode(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskOCRProcess, b, asynq.Queue(QueueOCR), asynq.MaxRetry(3))
	_, err = c.c.EnqueueContext(ctx, task)
	return err
}

func (c *Client) EnqueueExport(ctx context.Context, p ExportPayload) error {
	b, err := Encode(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskExportDocument, b, asynq.Queue(QueueExport), asynq.MaxRetry(2))
	_, err = c.c.EnqueueContext(ctx, task)
	return err
}
