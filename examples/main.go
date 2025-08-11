package main

import (
	"fmt"
	"log"
	"time"

	subzero "github.com/mr-alejandroo/subzero-client/internal"
)

func main() {
	fmt.Println("=== Subzero Client Examples ===")

	// GRPC
	fmt.Println("=== Basic gRPC Client Example ===")
	basicGRPCExample()

	fmt.Println("\n=== High-Performance gRPC Client Example ===")
	highPerformanceExample()

	// TCP
	fmt.Println("\n=== TCP Client Example ===")
	tcpClientExample()

	fmt.Println("\n=== Production Configuration Example ===")
	productionExample()

	fmt.Println("\n=== Integration Demo ===")
	runIntegrationDemo()

	fmt.Println("\n✅ All examples completed")
}

func basicGRPCExample() {
	// Default Config Client
	client, err := subzero.NewClient(subzero.DefaultConfig())
	if err != nil {
		log.Printf("Failed to create client: %v", err)
		return
	}
	defer client.Close()

	// Set a value
	err = client.Set("user:1001", []byte(`{"name":"Alice","age":30}`), nil)
	if err != nil {
		log.Printf("Set failed: %v", err)
		return
	}
	fmt.Println("✅ SET successful")

	// Get the value
	value, err := client.Get("user:1001")
	if err != nil {
		log.Printf("Get failed: %v", err)
		return
	}

	if value != nil {
		fmt.Printf("✅ GET successful: %s\n", string(value))
	} else {
		fmt.Println("❌ Key not found")
	}

	// Get with metadata
	value, metadata, err := client.GetWithMetadata("user:1001")
	if err != nil {
		log.Printf("Get with metadata failed: %v", err)
		return
	}

	if value != nil && metadata != nil {
		fmt.Printf("✅ GET with metadata: %s (size: %d bytes)\n", string(value), metadata.SizeBytes)
	}

	// Delete the key
	found, err := client.Delete("user:1001")
	if err != nil {
		log.Printf("Delete failed: %v", err)
		return
	}
	fmt.Printf("✅ DELETE successful (found: %t)\n", found)
}

func highPerformanceExample() {
	// High Perf
	config := &subzero.ClientConfig{
		Address:           "127.0.0.1:8080",
		MaxConnections:    8,
		ConnectTimeout:    2 * time.Second,
		RequestTimeout:    50 * time.Millisecond,
		KeepAliveTime:     30 * time.Second,
		KeepAliveTimeout:  5 * time.Second,
		MaxRetries:        5,
		InitialBackoff:    5 * time.Millisecond,
		MaxBackoff:        500 * time.Millisecond,
		BackoffMultiplier: 1.5,
		EnablePooling:     true,
		PoolReuse:         true,
		EnableTLS:         false,
	}

	client, err := subzero.NewClient(config)
	if err != nil {
		log.Printf("Failed to create high-performance client: %v", err)
		return
	}
	defer client.Close()

	// Test batch
	items := map[string][]byte{
		"session:abc123": []byte("active"),
		"config:timeout": []byte("30000"),
		"user:1002":      []byte(`{"name":"Bob","age":25}`),
		"cache:stats":    []byte(`{"hits":1000,"misses":50}`),
	}

	// Test TTL
	ttl := uint64(3600)
	err = client.BatchSet(items, &ttl)
	if err != nil {
		log.Printf("Batch set failed: %v", err)
		return
	}
	fmt.Println("✅ BATCH_SET successful")

	// Batch get
	keys := []string{"session:abc123", "config:timeout", "user:1002", "nonexistent"}
	results, err := client.BatchGet(keys)
	if err != nil {
		log.Printf("Batch get failed: %v", err)
		return
	}

	fmt.Printf("✅ BATCH_GET successful: found %d keys\n", len(results))
	for key, value := range results {
		fmt.Printf("   %s: %s\n", key, string(value))
	}

	// Health check
	health, err := client.Health()
	if err != nil {
		log.Printf("Health check failed: %v", err)
		return
	}
	fmt.Printf("✅ Health: %s (uptime: %ds, connections: %d)\n",
		health.Status, health.UptimeSeconds, health.ActiveConnections)
}

func tcpClientExample() {
	// TCP Client
	config := &subzero.TCPClientConfig{
		Address:        "127.0.0.1:8081",
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
		MaxRetries:     3,
		RetryDelay:     200 * time.Millisecond,
		KeepAlive:      true,
	}

	tcpClient, err := subzero.NewTCPClient(config)
	if err != nil {
		log.Printf("Failed to create TCP client: %v", err)
		return
	}
	defer tcpClient.Close()

	// Test connectivity
	err = tcpClient.Ping()
	if err != nil {
		log.Printf("Ping failed: %v", err)
		return
	}
	fmt.Println("✅ PING successful")

	// Set and get using TCP protocol
	err = tcpClient.Set("tcp:test", "hello world")
	if err != nil {
		log.Printf("TCP set failed: %v", err)
		return
	}
	fmt.Println("✅ TCP SET successful")

	value, err := tcpClient.Get("tcp:test")
	if err != nil {
		log.Printf("TCP get failed: %v", err)
		return
	}
	fmt.Printf("✅ TCP GET successful: %s\n", value)

	// Delete using TCP protocol
	found, err := tcpClient.Delete("tcp:test")
	if err != nil {
		log.Printf("TCP delete failed: %v", err)
		return
	}
	fmt.Printf("✅ TCP DELETE successful (found: %t)\n", found)
}

func productionExample() {
	config := subzero.ProductionConfig("cache.example.com:8080")
	config.TLSServerName = "cache.example.com"

	client, err := subzero.NewClient(config)
	if err != nil {
		log.Printf("Failed to create production client: %v", err)
		return
	}
	defer client.Close()

	// Check if connected
	if !client.IsConnected() {
		log.Printf("Client is not connected")
		return
	}
	fmt.Println("✅ Production client connected")

	clientConfig := client.GetConfig()
	fmt.Printf("✅ Client config: %d connections, %v timeout\n",
		clientConfig.MaxConnections, clientConfig.RequestTimeout)

	fmt.Println("✅ Production configuration validated")
}
