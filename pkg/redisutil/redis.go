package redisutil

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	client *redis.Client
	logger *slog.Logger
}

func NewClient(addr string, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}

	opts := &redis.Options{
		Addr:     addr,
		Password: "", // no password set
		DB:       0,  // use default DB
	}

	rdb := redis.NewClient(opts)
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	logger.Info("redis: berhasil terkoneksi", "addr", addr)
	return &Client{client: rdb, logger: logger}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) DB() *redis.Client {
	return c.client
}
