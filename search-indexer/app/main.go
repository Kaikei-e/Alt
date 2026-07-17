package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/redis/go-redis/v9"

	"search-indexer/bootstrap"
	"search-indexer/config"
	"search-indexer/consumer"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(runHealthcheck())
		case "provision-consumer-group":
			os.Exit(runProvisionConsumerGroup())
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := bootstrap.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// runHealthcheck performs a health check against the local HTTP server.
func runHealthcheck() int {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + config.HTTPAddr + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
	return 1
}

// runProvisionConsumerGroup creates the Redis Streams consumer group that
// search-indexer expects. Groups are provisioned here (or via mq-hub
// CreateConsumerGroup / scripts/provision-consumer-group.sh), not ad hoc
// inside Consumer.Start (DECREE §8).
func runProvisionConsumerGroup() int {
	cfg := consumer.ConfigFromEnv()
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision-consumer-group: parse redis url: %v\n", err)
		return 1
	}
	client := redis.NewClient(opts)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := consumer.ProvisionConsumerGroup(ctx, client, cfg.StreamKey, cfg.GroupName); err != nil {
		fmt.Fprintf(os.Stderr, "provision-consumer-group: %v\n", err)
		return 1
	}
	fmt.Printf("ok: stream=%s group=%s\n", cfg.StreamKey, cfg.GroupName)
	return 0
}
