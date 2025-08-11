# SubZero - GO Client

[![Go Reference](https://pkg.go.dev/badge/github.com/mr-alejandroo/subzero-client.svg)](https://pkg.go.dev/github.com/mr-alejandroo/subzero-client)
[![Go Report Card](https://goreportcard.com/badge/github.com/mr-alejandroo/subzero-client)](https://goreportcard.com/report/github.com/mr-alejandroo/subzero-client)

SubZero is a high-performance Go client for Subzero Server, a distributed disk-first cache system. It provides both gRPC and TCP protocols with comprehensive error handling, retry logic, and connection pooling.

## Features

- **Dual Protocol Support**: gRPC (recommended) and TCP text protocol
- **Connection Pooling**: Configurable connection pools for optimal performance
- **Automatic Retries**: Intelligent retry logic with exponential backoff
- **Comprehensive Error Handling**: Structured error types with detailed context
- **Production Ready**: Extensive configuration options and professional error handling
- **Type Safety**: Full type safety with protobuf-generated types
- **Performance Optimized**: Sub-millisecond latency for same-server deployment

## Installation

```bash
go get github.com/mr-alejandroo/subzero-client
```

## Quick Start

### Basic gRPC Client

```go
package main

import (
    "log"
    
    "github.com/mr-alejandroo/subzero-client"
)

func main() {
    // Create Client
    client, err := subzero.NewClient(subzero.DefaultConfig())
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Set Value
    err = client.Set("user:1001", []byte(`{"name":"Alice","age":30}`), nil)
    if err != nil {
        log.Fatal(err)
    }

    // Get Value
    value, err := client.Get("user:1001")
    if err != nil {
        log.Fatal(err)
    }

    if value != nil {
        log.Printf("Retrieved: %s", string(value))
    }
}
```


#### Delete Operation
```go
found, err := client.Delete("key")
if found {
    log.Println("Key was deleted")
} else {
    log.Println("Key was not found")
}
```

#### Health and Metrics
```go
// Health check
health, err := client.Health()
if err == nil {
    log.Printf("Status: %s, Uptime: %ds", health.Status, health.UptimeSeconds)
}

// Prometheus metrics
metrics, err := client.Metrics()
if err == nil {
    log.Printf("Metrics: %s", metrics)
}
```

### TCP Client

For simple integrations, use the TCP text protocol:

```go
// Create TCP client
tcpClient, err := subzero.NewTCPClient(subzero.DefaultTCPConfig())
if err != nil {
    log.Fatal(err)
}
defer tcpClient.Close()

// Test connectivity
err = tcpClient.Ping()

// Set and get
err = tcpClient.Set("key", "value")
value, err := tcpClient.Get("key")

// Delete
found, err := tcpClient.Delete("key")
```

## Error Handling

SubZero provides comprehensive error handling with structured error types:

```go
err := client.Set("", []byte("value"), nil)
if err != nil {
    if subzero.IsCacheError(err) {
        cacheErr := subzero.GetCacheError(err)
        log.Printf("Cache error: %s (code: %s, retryable: %t)", 
                  cacheErr.Message, cacheErr.Code, cacheErr.IsRetryable())
    }
    
    // Check specific error types
    if subzero.IsConnectionError(err) {
        log.Println("Connection error - check server availability")
    }
    
    if subzero.IsTimeoutError(err) {
        log.Println("Timeout error - consider increasing timeout")
    }
    
    if subzero.IsKeyNotFoundError(err) {
        log.Println("Key not found")
    }
}
```

## Configuration Options

### ClientConfig (gRPC)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Address` | `string` | `"127.0.0.1:8080"` | Server address |
| `MaxConnections` | `int` | `4` | Connection pool size |
| `ConnectTimeout` | `time.Duration` | `5s` | Connection timeout |
| `RequestTimeout` | `time.Duration` | `100ms` | Request timeout |
| `KeepAliveTime` | `time.Duration` | `10s` | Keep-alive interval |
| `KeepAliveTimeout` | `time.Duration` | `1s` | Keep-alive timeout |
| `MaxRetries` | `int` | `3` | Maximum retry attempts |
| `InitialBackoff` | `time.Duration` | `10ms` | Initial retry delay |
| `MaxBackoff` | `time.Duration` | `1s` | Maximum retry delay |
| `BackoffMultiplier` | `float64` | `2.0` | Backoff multiplier |
| `EnablePooling` | `bool` | `true` | Enable connection pooling |
| `PoolReuse` | `bool` | `true` | Enable context reuse |
| `EnableTLS` | `bool` | `false` | Enable TLS |

### TCPClientConfig (TCP)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Address` | `string` | `"127.0.0.1:8081"` | Server address |
| `ConnectTimeout` | `time.Duration` | `5s` | Connection timeout |
| `ReadTimeout` | `time.Duration` | `1s` | Read timeout |
| `WriteTimeout` | `time.Duration` | `1s` | Write timeout |
| `MaxRetries` | `int` | `3` | Maximum retry attempts |
| `RetryDelay` | `time.Duration` | `100ms` | Retry delay |
| `KeepAlive` | `bool` | `true` | Enable TCP keep-alive |

## Performance Comparisons

COMING SOON...

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.