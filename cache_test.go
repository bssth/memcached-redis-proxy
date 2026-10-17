package main

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/bssth/go-memcached"
	"github.com/redis/go-redis/v9"
)

// startProxy runs the proxy in-process on a random port, backed by miniredis,
// and returns a real memcached client connected to it and the proxy address.
func startProxy(t *testing.T, prefix string) (*miniredis.Miniredis, *memcache.Client, string) {
	t.Helper()

	backend := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: backend.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	server := memcached.NewServer(listener.Addr().String(), &Cache{client: client, prefix: prefix})
	go func() { _ = server.Serve(listener) }()

	mc := memcache.New(listener.Addr().String())
	mc.Timeout = 2 * time.Second

	return backend, mc, listener.Addr().String()
}

func mustGet(t *testing.T, mc *memcache.Client, key string) *memcache.Item {
	t.Helper()

	item, err := mc.Get(key)
	if err != nil {
		t.Fatalf("get %q: %v", key, err)
	}

	return item
}

func expectMiss(t *testing.T, mc *memcache.Client, key string) {
	t.Helper()

	if item, err := mc.Get(key); !errors.Is(err, memcache.ErrCacheMiss) {
		t.Fatalf("get %q = %v, %v; want a miss", key, item, err)
	}
}

func TestSetGetDelete(t *testing.T) {
	t.Parallel()
	_, mc, _ := startProxy(t, "")

	if err := mc.Set(&memcache.Item{Key: "foo", Value: []byte("bar")}); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, mc, "foo"); string(got.Value) != "bar" || got.Flags != 0 {
		t.Fatalf("got %q flags %d, want %q flags 0", got.Value, got.Flags, "bar")
	}
	if err := mc.Delete("foo"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	expectMiss(t, mc, "foo")
}

func TestDeleteMissingKeyIsNotFound(t *testing.T) {
	t.Parallel()
	_, mc, _ := startProxy(t, "")

	if err := mc.Delete("nope"); !errors.Is(err, memcache.ErrCacheMiss) {
		t.Fatalf("delete missing key = %v, want ErrCacheMiss (NOT_FOUND)", err)
	}
}

func TestFlagsAreKept(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	value := []byte("bin\x00\r\nary\xff")
	for _, flags := range []uint32{1, 42, 1<<32 - 1} {
		if err := mc.Set(&memcache.Item{Key: "k", Value: value, Flags: flags}); err != nil {
			t.Fatal(err)
		}
		got := mustGet(t, mc, "k")
		if !bytes.Equal(got.Value, value) || got.Flags != flags {
			t.Fatalf("got %q flags %d, want %q flags %d", got.Value, got.Flags, value, flags)
		}
	}

	if typ := backend.Type("k"); typ != "hash" {
		t.Fatalf("item with flags stored as %q, want hash", typ)
	}
}

func TestZeroFlagsStoredAsPlainString(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("v1"), Flags: 5}); err != nil {
		t.Fatal(err)
	}
	// Overwriting a hash with a plain value must work.
	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("v2")}); err != nil {
		t.Fatal(err)
	}

	got, err := backend.Get("k")
	if err != nil || got != "v2" {
		t.Fatalf("redis value = %q, %v; want plain string v2", got, err)
	}
	if item := mustGet(t, mc, "k"); string(item.Value) != "v2" || item.Flags != 0 {
		t.Fatalf("got %q flags %d, want v2 flags 0", item.Value, item.Flags)
	}
}

func TestReadsPlainRedisStrings(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	// Written by an older version of the proxy or by another Redis client.
	_ = backend.Set("legacy", "plain")

	if item := mustGet(t, mc, "legacy"); string(item.Value) != "plain" || item.Flags != 0 {
		t.Fatalf("got %q flags %d, want plain flags 0", item.Value, item.Flags)
	}
}

func TestOtherRedisTypesAreMisses(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	_, _ = backend.Push("list", "a")
	backend.HSet("foreign-hash", "x", "y")

	expectMiss(t, mc, "list")
	expectMiss(t, mc, "foreign-hash")
}

func TestExpiration(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	for _, flags := range []uint32{0, 9} {
		if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("v"), Flags: flags, Expiration: 10}); err != nil {
			t.Fatal(err)
		}
		if ttl := backend.TTL("k"); ttl <= 9*time.Second || ttl > 10*time.Second {
			t.Fatalf("flags %d: redis TTL = %v, want about 10s", flags, ttl)
		}
		backend.FastForward(11 * time.Second)
		expectMiss(t, mc, "k")
	}
}

func TestExpirationInThePastDeletesKey(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	_ = backend.Set("k", "old")

	// More than 30 days means an absolute Unix time; this one is an hour ago.
	past := int32(time.Now().Add(-time.Hour).Unix())
	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("new"), Expiration: past}); err != nil {
		t.Fatal(err)
	}

	expectMiss(t, mc, "k")
	if backend.Exists("k") {
		t.Fatal("key with an expiry in the past is still in Redis")
	}
}

func TestNegativeExpirationDeletesKey(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	_ = backend.Set("k", "old")

	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("new"), Expiration: -1}); err != nil {
		t.Fatal(err)
	}

	expectMiss(t, mc, "k")
	if backend.Exists("k") {
		t.Fatal("key with a negative expiration is still in Redis")
	}
}

func TestMultiGet(t *testing.T) {
	t.Parallel()
	_, mc, _ := startProxy(t, "")

	_ = mc.Set(&memcache.Item{Key: "plain", Value: []byte("p")})
	_ = mc.Set(&memcache.Item{Key: "flagged", Value: []byte("f"), Flags: 3})

	items, err := mc.GetMulti([]string{"plain", "missing", "flagged"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %v", len(items), items)
	}
	if got := items["plain"]; got == nil || string(got.Value) != "p" || got.Flags != 0 {
		t.Fatalf("plain = %+v", got)
	}
	if got := items["flagged"]; got == nil || string(got.Value) != "f" || got.Flags != 3 {
		t.Fatalf("flagged = %+v", got)
	}
}

func TestMalformedCommandDoesNotCrash(t *testing.T) {
	t.Parallel()
	_, mc, addr := startProxy(t, "")

	// These lines used to panic and kill the whole proxy.
	for _, line := range []string{"set abcdefgh", "set k 0 0 -5"} {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = conn.Write([]byte(line + "\r\n"))
		reply, err := bufio.NewReader(conn).ReadString('\n')
		_ = conn.Close()
		if err != nil || !strings.HasPrefix(reply, "CLIENT_ERROR") {
			t.Fatalf("%q: reply %q, %v; want CLIENT_ERROR", line, reply, err)
		}
	}

	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("still alive")}); err != nil {
		t.Fatalf("proxy stopped working: %v", err)
	}
}

func TestPrefix(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "mc:")

	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if got, err := backend.Get("mc:k"); err != nil || got != "v" {
		t.Fatalf("redis mc:k = %q, %v; want v", got, err)
	}
	if backend.Exists("k") {
		t.Fatal("key stored without prefix")
	}
}

func TestRedisErrors(t *testing.T) {
	t.Parallel()
	backend, mc, _ := startProxy(t, "")

	backend.SetError("ERR simulated failure")

	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("v")}); err == nil {
		t.Fatal("set succeeded while Redis fails, want SERVER_ERROR")
	}
	// A Redis failure must not look like a miss or NOT_FOUND.
	if _, err := mc.Get("k"); err == nil || errors.Is(err, memcache.ErrCacheMiss) {
		t.Fatalf("get while Redis fails = %v, want SERVER_ERROR", err)
	}
	if err := mc.Delete("k"); err == nil || errors.Is(err, memcache.ErrCacheMiss) {
		t.Fatalf("delete while Redis fails = %v, want SERVER_ERROR", err)
	}

	backend.SetError("")
	if err := mc.Set(&memcache.Item{Key: "k", Value: []byte("v")}); err != nil {
		t.Fatalf("set after Redis recovered: %v", err)
	}
}

func TestExpirationHelper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		item        memcached.Item
		wantTTL     bool
		wantExpired bool
	}{
		{name: "no expiry", item: memcached.Item{}},
		{name: "future", item: memcached.Item{Ttl: 60, Expires: time.Now().Add(time.Minute)}, wantTTL: true},
		{name: "past", item: memcached.Item{Ttl: -5, Expires: time.Now().Add(-5 * time.Second)}, wantExpired: true},
		{name: "sub-second left, Ttl rounded to 0", item: memcached.Item{Expires: time.Now().Add(500 * time.Millisecond)}, wantTTL: true},
		{name: "negative ttl without time", item: memcached.Item{Ttl: -1}, wantExpired: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ttl, expired := expiration(&test.item)
			if expired != test.wantExpired || (ttl > 0) != test.wantTTL {
				t.Fatalf("expiration() = %v, %v; want ttl>0 %v, expired %v", ttl, expired, test.wantTTL, test.wantExpired)
			}
		})
	}
}
