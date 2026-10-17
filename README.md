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

`get` and `gets` with one or more keys (CAS is not supported, so `gets` replies like `get`),
`set` and `delete` (both with `noreply`), `stats`, `version`, `quit`.

Malformed commands get `CLIENT_ERROR`, values over 1 MB get `SERVER_ERROR object too large for cache`.
If Redis fails, every command replies `SERVER_ERROR backend error`, so a failure never looks like a miss.

## How data is stored in Redis

- An item with flags `0` is a plain Redis string. Data written by other Redis clients is readable as well.
- An item with non-zero flags is a Redis hash with the fields `value` and `flags`.
  Many clients (PHP, Python, Java) use flags to mark serialized or compressed values, so they must survive a round trip.
- Expiration is set with Redis TTL. A negative expiration or a time in the past removes the key, like in memcached.

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
