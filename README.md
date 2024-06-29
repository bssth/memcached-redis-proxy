# Memcached Redis Proxy

This project is a **Memcached protocol** proxy server that uses Redis to store and retrieve data. It is written in Go.

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.

### Prerequisites

- Go 1.22.4 or later
- Redis server

### Installing

Clone the repository:

```bash
git clone https://github.com/bssth/memcached-redis-proxy.git
```

Navigate to the project directory:

```bash
cd memcached-redis-proxy
```

Install the dependencies:

```bash
go mod download
```

### Usage

You can start the server with the following command:

```bash
go run main.go
```

By default, the server listens on port **11211** and connects to a Redis server at **localhost:6379**. You can customize these settings with command-line flags:

```bash
go run main.go -port=11212 -redis=localhost:6380
```

**redis-password** and **redis-db** are also supported.

## Testing

You can run test with the following command:

```bash
go run testing/main.go
```

By default, test connects to a server at localhost:11211. You can customize this setting with a command-line flag:

```bash
go run testing/main.go -dsn=localhost:11212
```

## Contributing

Feel free to submit pull requests.

## License

See LICENSE.md for details.