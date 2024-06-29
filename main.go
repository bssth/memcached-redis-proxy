package main

import (
	"context"
	"flag"
	"github.com/bssth/go-memcached"
	"github.com/redis/go-redis/v9"
	"log"
	"strconv"
	"time"
)

type Cache struct {
	client *redis.Client
}

func (c Cache) Get(key string) memcached.MemcachedResponse {
	log.Println("Get", key)

	val := c.client.Get(context.Background(), key)
	if err := val.Err(); err != nil {
		log.Println(err)
		return nil
	}

	item, err := val.Bytes()
	if err != nil {
		log.Println(err)
		return nil
	}

	return &memcached.ItemResponse{
		Item: &memcached.Item{
			Key:   key,
			Value: item,
		},
	}
}

func (c Cache) Set(item *memcached.Item) memcached.MemcachedResponse {
	log.Println("Set", item.Key)
	c.client.Set(context.Background(), item.Key, item.Value, time.Second*time.Duration(item.Ttl))
	return nil
}

func (c Cache) Delete(key string) memcached.MemcachedResponse {
	log.Println("Delete", key)
	c.client.Del(context.Background(), key)
	return nil
}

var (
	port      = flag.Int("port", 11211, "Port to listen on")
	redisAddr = flag.String("redis", "localhost:6379", "Redis server address")
	redisPass = flag.String("redis-pass", "", "Redis password")
	redisDb   = flag.Int("redis-db", 0, "Redis DB")
)

func main() {
	flag.Parse()

	cache := &Cache{}
	cache.client = redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: *redisPass,
		DB:       *redisDb,
	})

	if status := cache.client.Ping(context.Background()); status.Err() != nil {
		log.Fatalln("Cannot connect to Redis:", status.Err())
	}
	log.Println("Connected to Redis")

	log.Println("Starting memcached proxy server on port", *port)
	server := memcached.NewServer(":"+strconv.Itoa(*port), cache)
	err := server.ListenAndServe()
	if err != nil {
		log.Fatalln(err)
	}
}
