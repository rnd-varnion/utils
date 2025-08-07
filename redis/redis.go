package redis

import (
	"context"
	"os"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/rnd-varnion/utils/logger"
)

var (
	REDIS_HOST     = "REDIS_HOST"
	REDIS_USERNAME = "REDIS_USERNAME"
	REDIS_PASSWORD = "REDIS_PASSWORD"
)

var RedisClient0 *redis.Client
var RedisClient1 *redis.Client

func Ping(ctx context.Context, db *redis.Client) error {
	ping := db.Ping(ctx)
	if ping.Err() != nil {
		return ping.Err()
	}

	return nil
}

func InitRedis() (*redis.Client, *redis.Client) {
	_ = godotenv.Load()

	addr := os.Getenv(REDIS_HOST)
	if addr == "" {
		addr = "localhost:6379" // fallback
	}
	username := os.Getenv(REDIS_USERNAME)
	password := os.Getenv(REDIS_PASSWORD)

	logger.Log.Infof("[INFO] Connecting to Redis at %s\n", addr)

	RedisClient0 = redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username,
		Password: password,
		DB:       0,
	})

	RedisClient1 = redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username,
		Password: password,
		DB:       1,
	})

	return RedisClient0, RedisClient1
}
