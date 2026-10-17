package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bssth/go-memcached"
	"github.com/redis/go-redis/v9"
)

// Items with non-zero flags are stored as a Redis hash with these fields.
// Items with zero flags are stored as plain Redis strings, so data written
// by older versions of the proxy (or by other Redis clients) stays readable.
const (
	fieldValue = "value"
	fieldFlags = "flags"
)

// Cache serves memcached get/set/delete commands from Redis.
type Cache struct {
	client  *redis.Client
	prefix  string
	verbose bool
}

// serverError makes the memcached server reply "SERVER_ERROR <reason>".
type serverError struct {
	reason string
}

func (e *serverError) WriteResponse(w io.Writer) {
	_, _ = fmt.Fprintf(w, "SERVER_ERROR %s\r\n", e.reason)
}

// notFound is any non-nil response: the memcached server then replies NOT_FOUND to delete.
type notFound struct{}

func (notFound) WriteResponse(io.Writer) {}

func (c *Cache) key(key string) string {
	return c.prefix + key
}

func (c *Cache) debugf(format string, args ...any) {
	if c.verbose {
		log.Printf(format, args...)
	}
}

// Get returns nil for a miss. A Redis error is also reported as a miss,
// because go-memcached always ends a get reply with END and cannot send an error instead.
func (c *Cache) Get(key string) memcached.MemcachedResponse {
	c.debugf("get %q", key)
	ctx := context.Background()

	value, err := c.client.Get(ctx, c.key(key)).Bytes()
	switch {
	case err == nil:
		return itemResponse(key, value, 0)
	case errors.Is(err, redis.Nil):
		return nil
	case isWrongType(err):
		return c.getWithFlags(ctx, key)
	default:
		log.Printf("get %q: %v", key, err)
		return nil
	}
}

func (c *Cache) getWithFlags(ctx context.Context, key string) memcached.MemcachedResponse {
	fields, err := c.client.HMGet(ctx, c.key(key), fieldValue, fieldFlags).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) && !isWrongType(err) {
			log.Printf("get %q: %v", key, err)
		}
		return nil
	}

	value, ok := fields[0].(string)
	if !ok {
		return nil // not a hash written by this proxy
	}
	var flags uint64
	if flagsText, ok := fields[1].(string); ok {
		flags, err = strconv.ParseUint(flagsText, 10, 32)
		if err != nil {
			log.Printf("get %q: bad flags %q", key, flagsText)
			return nil
		}
	}

	return itemResponse(key, []byte(value), int(flags))
}

func itemResponse(key string, value []byte, flags int) memcached.MemcachedResponse {
	return &memcached.ItemResponse{
		Item: &memcached.Item{Key: key, Value: value, Flags: flags},
	}
}

// Set returns nil for success (the server replies STORED) or a SERVER_ERROR.
func (c *Cache) Set(item *memcached.Item) memcached.MemcachedResponse {
	c.debugf("set %q flags=%d ttl=%d bytes=%d", item.Key, item.Flags, item.Ttl, len(item.Value))
	ctx := context.Background()
	key := c.key(item.Key)

	ttl, expired := expiration(item)
	if expired {
		// In memcached an item with an expiry in the past is stored and immediately gone.
		if err := c.client.Del(ctx, key).Err(); err != nil {
			return c.setFailed(item.Key, err)
		}
		return nil
	}

	var err error
	if item.Flags == 0 {
		// SET replaces a key of any type, including a hash written with flags.
		err = c.client.Set(ctx, key, item.Value, ttl).Err()
	} else {
		_, err = c.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, key)
			pipe.HSet(ctx, key, fieldValue, item.Value, fieldFlags, strconv.Itoa(item.Flags))
			if ttl > 0 {
				pipe.PExpire(ctx, key, ttl)
			}
			return nil
		})
	}
	if err != nil {
		return c.setFailed(item.Key, err)
	}

	return nil
}

func (c *Cache) setFailed(key string, err error) memcached.MemcachedResponse {
	log.Printf("set %q: %v", key, err)
	return &serverError{reason: "backend error"}
}

// expiration turns the item's memcached expiry into a Redis TTL.
// A zero TTL means "never expires". expired is true when the expiry is already in the past.
func expiration(item *memcached.Item) (ttl time.Duration, expired bool) {
	if item.Expires.IsZero() {
		return 0, item.Ttl < 0
	}

	ttl = time.Until(item.Expires)
	if ttl <= 0 {
		return 0, true
	}

	return ttl, false
}

// Delete returns nil when the key was deleted (DELETED) and non-nil otherwise (NOT_FOUND).
// go-memcached can't send SERVER_ERROR for delete, so a Redis error is logged and reported as NOT_FOUND.
func (c *Cache) Delete(key string) memcached.MemcachedResponse {
	c.debugf("delete %q", key)

	deleted, err := c.client.Del(context.Background(), c.key(key)).Result()
	if err != nil {
		log.Printf("delete %q: %v", key, err)
		return notFound{}
	}
	if deleted == 0 {
		return notFound{}
	}

	return nil
}

func isWrongType(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "WRONGTYPE")
}
