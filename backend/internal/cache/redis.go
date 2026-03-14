package cache

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client
var Ctx = context.Background()

func Connect() {
	Client = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s",
			os.Getenv("REDIS_HOST"),
			os.Getenv("REDIS_PORT"),
		),
	})

	_, err := Client.Ping(Ctx).Result()
	if err != nil {
		log.Fatal("❌ Redis connection failed:", err)
	}
	log.Println("✅ Redis connected successfully!")
}

// Set a value with expiry
func Set(key string, value interface{}, expiry time.Duration) error {
	return Client.Set(Ctx, key, value, expiry).Err()
}

// Get a value
func Get(key string) (string, error) {
	return Client.Get(Ctx, key).Result()
}

// Delete a key
func Delete(key string) error {
	return Client.Del(Ctx, key).Err()
}

// Check if key exists
func Exists(key string) bool {
	result, _ := Client.Exists(Ctx, key).Result()
	return result > 0
}

// Increment (for rate limiting)
func Increment(key string, expiry time.Duration) (int64, error) {
	pipe := Client.Pipeline()
	incr := pipe.Incr(Ctx, key)
	pipe.Expire(Ctx, key, expiry)
	_, err := pipe.Exec(Ctx)
	return incr.Val(), err
}