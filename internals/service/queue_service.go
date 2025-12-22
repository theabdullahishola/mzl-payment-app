package service

import (
	"context"
	"time"
)

type QueueService interface {

	Enqueue(ctx context.Context, queueName string, payload []byte) error
	StartWorker(ctx context.Context, queueName string)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
    Get(ctx context.Context, key string, dest interface{}) error
    Delete(ctx context.Context, key string) error
	TryLockIdempotencyKey(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Ping(ctx context.Context) error
}