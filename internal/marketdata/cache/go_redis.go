package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisCommander 是 Cache 所需的最小 go-redis 命令集合；连接池生命周期由调用方管理。
type redisCommander interface {
	Get(context.Context, string) *redis.StringCmd
	Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
	Ping(context.Context) *redis.StatusCmd
}

// GoRedis 将 Root Server 持有的 go-redis Client 适配为 Data Cache；不拥有、不关闭 Client。
type GoRedis struct {
	client redisCommander
}

// NewGoRedis 创建共享 go-redis Cache Adapter；Client 由 Root Server 创建并负责关闭。
func NewGoRedis(client redisCommander) (*GoRedis, error) {
	if client == nil {
		return nil, errors.New("go-redis client is required")
	}
	return &GoRedis{client: client}, nil
}

func (r *GoRedis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if r == nil || r.client == nil {
		return nil, false, errors.New("go-redis client is not configured")
	}
	value, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (r *GoRedis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if r == nil || r.client == nil {
		return errors.New("go-redis client is not configured")
	}
	if ttl.Milliseconds() < 1 {
		return errors.New("Redis TTL must be at least 1ms")
	}
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *GoRedis) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("go-redis client is not configured")
	}
	return r.client.Ping(ctx).Err()
}
