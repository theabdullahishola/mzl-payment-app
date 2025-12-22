package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/theabdullahishola/mzl-payment-app/internals/service"
)

const maxRetries = 3

type job struct {
	Payload []byte `json:"payload"`
	Retries int    `json:"retries"`
}

type RedisQueue struct {
	client         *redis.Client
	paymentService service.PaymentService
	logger         *slog.Logger
}

func NewRedisQueue(addr, password string, logger *slog.Logger) (*RedisQueue, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisQueue{
		client: rdb,
		logger: logger,
	}, nil
}

func (q *RedisQueue) SetPaymentService(svc service.PaymentService) {
	q.paymentService = svc
}

func (q *RedisQueue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

func (q *RedisQueue) Enqueue(ctx context.Context, queueName string, payload []byte) error {
	j := job{Payload: payload, Retries: 0}

	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return q.client.RPush(ctx, queueName, data).Err()
}

func (q *RedisQueue) StartWorker(ctx context.Context, queueName string) {
    q.logger.Info("Worker started", "queue", queueName)

    for {
        select {
        case <-ctx.Done():
            q.logger.Info("Worker shutting down...")
            return
        default:
            result, err := q.client.BLPop(ctx, 0, queueName).Result()
            if err != nil {
                if err != context.Canceled {
                    q.logger.Error("redis connection error", "error", err)
                }
                time.Sleep(time.Second)
                continue
            }

            var j job
            if err := json.Unmarshal([]byte(result[1]), &j); err != nil {
                q.logger.Error("skipping corrupt job", "error", err)
                continue
            }
            err = q.paymentService.ProcessPaystackEvent(context.Background(), j.Payload)

            if err != nil {
                q.logger.Warn("job failed", "retries", j.Retries, "error", err)
                q.handleFailure(ctx, queueName, j)
            } else {
                q.logger.Info("Job processed successfully")
            }
        }
    }
}

func (q *RedisQueue) handleFailure(ctx context.Context, queueName string, j job) {
	j.Retries++

	data, _ := json.Marshal(j)

	if j.Retries < maxRetries {
		q.logger.Info("Re-queueing job for retry")
		q.client.RPush(ctx, queueName, data)
	} else {
		dlqName := queueName + ":dead_letter"
		q.logger.Error("Job moved to DLQ", "queue", dlqName)
		q.client.RPush(ctx, dlqName, data)
	}
}


func (q *RedisQueue) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return q.client.Set(ctx, key, data, ttl).Err()
}

func (q *RedisQueue) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := q.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return fmt.Errorf("cache miss")
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (q *RedisQueue) Delete(ctx context.Context, key string) error {
	return q.client.Del(ctx, key).Err()
}


func (q *RedisQueue) TryLockIdempotencyKey(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	fullKey := fmt.Sprintf("idemp:%s", key)
	return q.client.SetNX(ctx, fullKey, "processing", ttl).Result()
}


func (q *RedisQueue) SetIdempotencyResult(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	fullKey := fmt.Sprintf("idemp:%s", key)
	return q.Set(ctx, fullKey, value, ttl)
}

func (q *RedisQueue) IsRateLimited(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	fullKey := fmt.Sprintf("ratelimit:%s", key)

	count, err := q.client.Incr(ctx, fullKey).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		q.client.Expire(ctx, fullKey, window)
	}
	if int(count) > limit {
		return true, nil
	}

	return false, nil
}