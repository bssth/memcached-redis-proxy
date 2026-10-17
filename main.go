package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/bssth/go-memcached"
	"github.com/redis/go-redis/v9"
)

var (
	port      = flag.Int("port", 11211, "Port to listen on")
	redisAddr = flag.String("redis", "localhost:6379", "Redis server address")
	redisPass = flag.String("redis-pass", "", "Redis password")
	redisDB   = flag.Int("redis-db", 0, "Redis DB")
	prefix    = flag.String("prefix", "", "Prefix added to every key in Redis")
	verbose   = flag.Bool("verbose", false, "Log every command")
)

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: *redisPass,
		DB:       *redisDB,
	})
	defer func() { _ = client.Close() }()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalln("Cannot connect to Redis:", err)
	}
	log.Println("Connected to Redis")

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("Starting memcached proxy server on port", *port)

	cache := &Cache{client: client, prefix: *prefix, verbose: *verbose}
	server := memcached.NewServer(listener.Addr().String(), cache)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case err := <-serveErr:
		log.Fatalln(err)
	case <-ctx.Done():
		log.Println("Shutting down")
		_ = listener.Close()
		if err := <-serveErr; err != nil && !errors.Is(err, net.ErrClosed) {
			log.Println(err)
		}
	}
}
