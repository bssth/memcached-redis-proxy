package main

import (
	"flag"
	"github.com/bradfitz/gomemcache/memcache"
	"log"
)

func main() {
	dsn := flag.String("dsn", "localhost:11211", "Server connection string")

	mc := memcache.New(*dsn)
	err := mc.Set(&memcache.Item{Key: "foo", Value: []byte("my value")})
	if err != nil {
		log.Fatalln("Cannot set")
	}

	it, err := mc.Get("foo")
	if err != nil {
		log.Fatalln("Cannot get")
	}

	if string(it.Value) != "my value" {
		log.Fatalf("expected value, got %s\n", it.Value)
	}

	err = mc.Delete("foo")
	if err != nil {
		log.Fatalln("Cannot delete")
	}

	log.Println("Test passed")
}
