package limiter

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const tokenBucketScript = `local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local state = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(state[1])
local ts = tonumber(state[2])
if tokens == nil or ts == nil then
  tokens = burst
  ts = now
end
local delta = now - ts
if delta < 0 then
  delta = 0
end
tokens = tokens + delta * rate / 1000.0
if tokens > burst then
  tokens = burst
end
local allowed = 0
if tokens >= 1.0 then
  tokens = tokens - 1.0
  allowed = 1
end
redis.call('HSET', key, 'tokens', tokens, 'ts', now)
redis.call('PEXPIRE', key, math.ceil(burst / rate * 2000.0) + 1000)
return allowed`

type evaler interface {
	Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)
	Close() error
}

type redisEvaler struct {
	client *redis.Client
}

func (r *redisEvaler) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return r.client.Eval(ctx, script, keys, args...).Result()
}

func (r *redisEvaler) Close() error {
	return r.client.Close()
}

type Redis struct {
	client  evaler
	prefix  string
	rate    float64
	burst   int
	timeout time.Duration
}

func NewRedis(addr, password string, requestsPerMinute, burst int) *Redis {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})
	return newRedis(&redisEvaler{client: client}, requestsPerMinute, burst)
}

func newRedis(client evaler, requestsPerMinute, burst int) *Redis {
	return &Redis{
		client:  client,
		prefix:  "ratelimit:",
		rate:    float64(requestsPerMinute) / 60.0,
		burst:   burst,
		timeout: 2 * time.Second,
	}
}

func (r *Redis) Allow(key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	res, err := r.client.Eval(ctx, tokenBucketScript, []string{r.prefix + key}, r.rate, r.burst, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	n, ok := res.(int64)
	return ok && n == 1, nil
}

func (r *Redis) Close() error {
	return r.client.Close()
}
