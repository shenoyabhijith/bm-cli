package redis

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
)

var ctx = context.Background()

func NewClient() *redis.Client {
	// Load environment variables
	godotenv.Load()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	dbStr := os.Getenv("REDIS_DB")
	db := 0
	if dbStr != "" {
		if d, err := strconv.Atoi(dbStr); err == nil {
			db = d
		}
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	return client
}
