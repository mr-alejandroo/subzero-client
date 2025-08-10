package subzero

import (
	"fmt"
	"testing"
	"time"
)

func TestIntegrationLocalServer(t *testing.T) {
	// Skip if running in short mode (CI/CD environments)
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires running Nova Cache server")
	}

	config := &ClientConfig{
		Address:           "127.0.0.1:8080",
		MaxConnections:    4,
		ConnectTimeout:    2 * time.Second,
		RequestTimeout:    1 * time.Second,
		KeepAliveTime:     10 * time.Second,
		KeepAliveTimeout:  time.Second,
		MaxRetries:        2,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		EnablePooling:     true,
		PoolReuse:         true,
		EnableTLS:         false,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Health Check
	t.Run("HealthCheck", func(t *testing.T) {
		start := time.Now()
		health, err := client.Health()
		latency := time.Since(start)

		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}

		t.Logf("✅ Health check successful")
		t.Logf("   Status: %s", health.Status)
		t.Logf("   Uptime: %d seconds", health.UptimeSeconds)
		t.Logf("   Active connections: %d", health.ActiveConnections)
		t.Logf("   Health check latency: %v", latency)

		if latency > 100*time.Millisecond {
			t.Logf("⚠️  Health check latency higher than expected: %v", latency)
		}
	})

	// Test basic Set/Get operations
	t.Run("BasicSetGet", func(t *testing.T) {
		testKey := fmt.Sprintf("test:integration:%d", time.Now().UnixNano())
		testValue := []byte(`{"message":"Hello Nova Cache","timestamp":"` + time.Now().Format(time.RFC3339) + `","test":true}`)

		// Test SET operation
		setStartNano := time.Now().UnixNano()
		err := client.Set(testKey, testValue, nil)
		setEndNano := time.Now().UnixNano()
		setLatency := time.Duration(setEndNano - setStartNano)

		if err != nil {
			t.Fatalf("SET operation failed: %v", err)
		}

		t.Logf("✅ SET operation successful")
		t.Logf("   Key: %s", testKey)
		t.Logf("   Value size: %d bytes", len(testValue))
		t.Logf("   SET latency: %v", setLatency)

		// Test GET operation
		getStart := time.Now()
		retrievedValue, err := client.Get(testKey)
		getLatency := time.Since(getStart)

		if err != nil {
			t.Fatalf("GET operation failed: %v", err)
		}

		if retrievedValue == nil {
			t.Fatalf("Expected value but got nil")
		}

		// Verify data integrity
		if string(retrievedValue) != string(testValue) {
			t.Fatalf("Data mismatch!\nExpected: %s\nGot: %s", string(testValue), string(retrievedValue))
		}

		t.Logf("✅ GET operation successful")
		t.Logf("   Retrieved value matches expected data")
		t.Logf("   GET latency: %v", getLatency)

		// Performance assessment
		totalLatency := setLatency + getLatency
		t.Logf("📊 Performance Summary:")
		t.Logf("   SET latency: %v", setLatency)
		t.Logf("   GET latency: %v", getLatency)
		t.Logf("   Total round-trip: %v", totalLatency)
		t.Logf("📊 Performance Summary:")
		t.Logf("   SET latency: %v", setLatency)
		t.Logf("   GET latency: %v", getLatency)
		t.Logf("   Total round-trip: %v", totalLatency)

		if setLatency < 5*time.Millisecond {
			t.Logf("🚀 Excellent SET performance: %v", setLatency)
		} else if setLatency < 50*time.Millisecond {
			t.Logf("✅ Good SET performance: %v", setLatency)
		} else {
			t.Logf("⚠️  SET latency higher than expected: %v", setLatency)
		}

		if getLatency < 5*time.Millisecond {
			t.Logf("🚀 Excellent GET performance: %v", getLatency)
		} else if getLatency < 50*time.Millisecond {
			t.Logf("✅ Good GET performance: %v", getLatency)
		} else {
			t.Logf("⚠️  GET latency higher than expected: %v", getLatency)
		}

		// Cleanup
		deleteStartNano := time.Now().UnixNano()
		found, err := client.Delete(testKey)
		deleteEndNano := time.Now().UnixNano()
		deleteLatency := time.Duration(deleteEndNano - deleteStartNano)

		if err != nil {
			t.Fatalf("DELETE operation failed: %v", err)
		}

		if !found {
			t.Fatalf("Expected to find key for deletion, but it was not found")
		}

		t.Logf("✅ DELETE operation successful")
		t.Logf("   DELETE latency: %v", deleteLatency)
	})

	// Test operations with metadata
	t.Run("GetWithMetadata", func(t *testing.T) {
		testKey := fmt.Sprintf("test:metadata:%d", time.Now().UnixNano())
		testValue := []byte("test data with metadata")
		ttl := uint64(3600) // 1 hour

		// Set with TTL
		err := client.Set(testKey, testValue, &ttl)
		if err != nil {
			t.Fatalf("SET with TTL failed: %v", err)
		}

		// Get with metadata
		startNano := time.Now().UnixNano()
		value, metadata, err := client.GetWithMetadata(testKey)
		endNano := time.Now().UnixNano()
		latency := time.Duration(endNano - startNano)

		if err != nil {
			t.Fatalf("GET with metadata failed: %v", err)
		}

		if value == nil {
			t.Fatalf("Expected value but got nil")
		}

		if metadata == nil {
			t.Fatalf("Expected metadata but got nil")
		}

		// Verify data
		if string(value) != string(testValue) {
			t.Fatalf("Data mismatch with metadata")
		}

		t.Logf("✅ GET with metadata successful")
		t.Logf("   Value size: %d bytes", len(value))
		t.Logf("   Metadata size: %d bytes", metadata.SizeBytes)
		t.Logf("   Created at: %d", metadata.CreatedAt)
		t.Logf("   Updated at: %d", metadata.UpdatedAt)
		if metadata.ExpiresAt != nil {
			t.Logf("   Expires at: %d", *metadata.ExpiresAt)
		}
		t.Logf("   Checksum: %s", metadata.Checksum)
		t.Logf("   GET with metadata latency: %v", latency)

		// Cleanup
		client.Delete(testKey)
	})

	// Test batch operations performance
	t.Run("BatchOperations", func(t *testing.T) {
		batchSize := 10
		items := make(map[string][]byte)
		keys := make([]string, 0, batchSize)

		// Prepare test data
		for i := 0; i < batchSize; i++ {
			key := fmt.Sprintf("test:batch:%d:%d", time.Now().UnixNano(), i)
			value := []byte(fmt.Sprintf(`{"batch_item":%d,"data":"test_%d","timestamp":"%s"}`, i, i, time.Now().Format(time.RFC3339)))
			items[key] = value
			keys = append(keys, key)
		}

		// Test BatchSet
		batchSetStartNano := time.Now().UnixNano()
		err := client.BatchSet(items, nil)
		batchSetEndNano := time.Now().UnixNano()
		batchSetLatency := time.Duration(batchSetEndNano - batchSetStartNano)

		if err != nil {
			t.Fatalf("BatchSet failed: %v", err)
		}

		t.Logf("✅ BatchSet successful")
		t.Logf("   Items: %d", batchSize)
		t.Logf("   BatchSet latency: %v", batchSetLatency)
		t.Logf("   Average per item: %v", batchSetLatency/time.Duration(batchSize))

		// Test BatchGet
		batchGetStartNano := time.Now().UnixNano()
		results, err := client.BatchGet(keys)
		batchGetEndNano := time.Now().UnixNano()
		batchGetLatency := time.Duration(batchGetEndNano - batchGetStartNano)

		if err != nil {
			t.Fatalf("BatchGet failed: %v", err)
		}

		if len(results) != batchSize {
			t.Fatalf("Expected %d results, got %d", batchSize, len(results))
		}

		// Verify all data
		for key, expectedValue := range items {
			retrievedValue, exists := results[key]
			if !exists {
				t.Fatalf("Missing key in batch results: %s", key)
			}
			if string(retrievedValue) != string(expectedValue) {
				t.Fatalf("Data mismatch for key %s", key)
			}
		}

		t.Logf("✅ BatchGet successful")
		t.Logf("   Retrieved items: %d", len(results))
		t.Logf("   BatchGet latency: %v", batchGetLatency)
		t.Logf("   Average per item: %v", batchGetLatency/time.Duration(batchSize))

		// Performance comparison
		totalBatchLatency := batchSetLatency + batchGetLatency
		t.Logf("📊 Batch Performance Summary:")
		t.Logf("   BatchSet: %v (%v per item)", batchSetLatency, batchSetLatency/time.Duration(batchSize))
		t.Logf("   BatchGet: %v (%v per item)", batchGetLatency, batchGetLatency/time.Duration(batchSize))
		t.Logf("   Total batch round-trip: %v", totalBatchLatency)

		// Cleanup
		for _, key := range keys {
			client.Delete(key)
		}
	})

	// Test performance under rapid operations
	t.Run("RapidOperations", func(t *testing.T) {
		numOps := 100
		keys := make([]string, numOps)

		// Prepare keys
		for i := 0; i < numOps; i++ {
			keys[i] = fmt.Sprintf("test:rapid:%d:%d", time.Now().UnixNano(), i)
		}

		// Rapid SET operations
		rapidSetStartNano := time.Now().UnixNano()
		for i, key := range keys {
			value := []byte(fmt.Sprintf("rapid_test_value_%d", i))
			err := client.Set(key, value, nil)
			if err != nil {
				t.Fatalf("Rapid SET operation %d failed: %v", i, err)
			}
		}
		rapidSetEndNano := time.Now().UnixNano()
		rapidSetLatency := time.Duration(rapidSetEndNano - rapidSetStartNano)

		// Rapid GET operations
		rapidGetStartNano := time.Now().UnixNano()
		for _, key := range keys {
			_, err := client.Get(key)
			if err != nil {
				t.Fatalf("Rapid GET operation failed for key %s: %v", key, err)
			}
		}
		rapidGetEndNano := time.Now().UnixNano()
		rapidGetLatency := time.Duration(rapidGetEndNano - rapidGetStartNano)

		t.Logf("✅ Rapid operations successful")
		t.Logf("   Operations: %d SET + %d GET", numOps, numOps)
		t.Logf("   Rapid SET total: %v (avg: %v)", rapidSetLatency, rapidSetLatency/time.Duration(numOps))
		t.Logf("   Rapid GET total: %v (avg: %v)", rapidGetLatency, rapidGetLatency/time.Duration(numOps))

		opsPerSecond := float64(numOps*2) / (rapidSetLatency.Seconds() + rapidGetLatency.Seconds())
		t.Logf("   Operations per second: %.0f", opsPerSecond)

		if opsPerSecond > 10000 {
			t.Logf("🚀 Excellent throughput: %.0f ops/sec", opsPerSecond)
		} else if opsPerSecond > 1000 {
			t.Logf("✅ Good throughput: %.0f ops/sec", opsPerSecond)
		} else {
			t.Logf("⚠️  Lower than expected throughput: %.0f ops/sec", opsPerSecond)
		}

		// Cleanup
		for _, key := range keys {
			client.Delete(key)
		}
	})

	t.Logf("🎉 All integration tests completed successfully!")
}
