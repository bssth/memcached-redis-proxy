# Memcached Redis Proxy

[![CI](https://github.com/bssth/memcached-redis-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/bssth/memcached-redis-proxy/actions/workflows/ci.yml)

This project is a **Memcached protocol** proxy server that uses Redis to store and retrieve data. It is written in Go.

## Getting Started

### Prerequisites

- Go 1.26 or later
- Redis server

### Installing

```bash
git clone https://github.com/bssth/memcached-redis-proxy.git
cd memcached-redis-proxy
go mod download
```

### Usage

```bash
go run .
```

By default, the server listens on port **11211** and connects to Redis at **localhost:6379**.

| Flag | Default | Description |
| --- | --- | --- |
| `-port` | `11211` | Port to listen on |
| `-redis` | `localhost:6379` | Redis server address |
| `-redis-pass` | | Redis password |
| `-redis-db` | `0` | Redis DB number |
| `-prefix` | | Prefix added to every key in Redis, e.g. `mc:` |
| `-verbose` | `false` | Log every command |

```bash
go run . -port=11212 -redis=localhost:6380 -prefix=mc:
```

The server stops cleanly on `SIGINT` / `SIGTERM`.

## Supported commands

`get`, `gets` (without CAS), `set`, `delete`, `stats`, `quit`.

## How data is stored in Redis

- An item with flags `0` is a plain Redis string. Data written by other Redis clients is readable as well.
- An item with non-zero flags is a Redis hash with the fields `value` and `flags`.
  Many clients (PHP, Python, Java) use flags to mark serialized or compressed values, so they must survive a round trip.
- Expiration is set with Redis TTL. An expiration time in the past removes the key, like in memcached.

## Known limitations

These come from the protocol parser in [bssth/go-memcached](https://github.com/bssth/go-memcached) and need to be fixed there:

- A malformed `set` line (for example `set abcdefgh` or a negative length) crashes the whole process.
- `get a b c` (multi-get) is treated as one key `"a b c"` and is always a miss.
- A negative relative expiration (for example `-1`) is treated as "never expires" instead of "already expired".
- If Redis fails, `get` replies with a miss and `delete` with `NOT_FOUND`, because the parser can't send `SERVER_ERROR` for them. `set` does reply `SERVER_ERROR`.

## Testing

Unit tests run the proxy in-process against an in-memory Redis ([miniredis](https://github.com/alicebob/miniredis)):

```bash
go test -race ./...
```

Smoke test against a running proxy:

```bash
go run ./testing -dsn=localhost:11211
```

## Contributing

Feel free to submit pull requests.

## License

See LICENSE.md for details.
