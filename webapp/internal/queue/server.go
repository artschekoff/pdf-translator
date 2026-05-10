package queue

import (
	"context"
	"log"

	"github.com/hibiken/asynq"
)

func NewServer(redisURL string, concurrency int) *asynq.Server {
	opt, _ := asynq.ParseRedisURI(redisURL)
	return asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			QueueOCR:    10,
			QueueAI:     5,
			QueueExport: 1,
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			log.Printf("asynq error: task=%s err=%v", task.Type(), err)
		}),
	})
}

func NewMux() *asynq.ServeMux {
	return asynq.NewServeMux()
}
