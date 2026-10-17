module github.com/bssth/memcached-redis-proxy

go 1.26.0

require github.com/bradfitz/gomemcache v0.0.0-20260422231931-4d751bb6e37c

require (
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/bssth/go-memcached v0.0.0-20240629150313-6351b3c57773
	github.com/redis/go-redis/v9 v9.23.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)
