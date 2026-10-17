// Command testing is a quick smoke test against a running proxy.
package main

import (
	"flag"
	"log"

	"github.com/bradfitz/gomemcache/memcache"
)

func main() {
	dsn := flag.String("dsn", "localhost:11211", "Server connection string")
	flag.Parse()

	mc := memcache.New(*dsn)
	err := mc.Set(&memcache.Item{Key: "foo", Value: []byte("my value"), Flags: 7})
	if err != nil {
		log.Fatalln("Cannot set:", err)
	}

	it, err := mc.Get("foo")
	if err != nil {
		log.Fatalln("Cannot get:", err)
	}

	if string(it.Value) != "my value" || it.Flags != 7 {
		log.Fatalf("expected %q with flags 7, got %q with flags %d\n", "my value", it.Value, it.Flags)
	}

	err = mc.Delete("foo")
	if err != nil {
		log.Fatalln("Cannot delete:", err)
	}

	if _, err := mc.Get("foo"); err != memcache.ErrCacheMiss {
		log.Fatalln("Expected a miss after delete, got:", err)
	}

	log.Println("Test passed")
}
