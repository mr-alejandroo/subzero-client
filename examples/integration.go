package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	subzero "github.com/mr-alejandroo/subzero-client/internal"
)

// Example
type User struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Age      int    `json:"age"`
	LastSeen int64  `json:"last_seen"`
}

// Example
type SessionData struct {
	UserID    int    `json:"user_id"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	IP        string `json:"ip"`
}

// Example
type CacheService struct {
	client *subzero.Client
}

func NewCacheService(config *subzero.ClientConfig) (*CacheService, error) {
	client, err := subzero.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache client: %w", err)
	}

	return &CacheService{client: client}, nil
}

func (cs *CacheService) Close() error {
	return cs.client.Close()
}

func (cs *CacheService) SetUser(user *User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	key := fmt.Sprintf("user:%d", user.ID)
	ttl := uint64(3600) // 1 hour

	return cs.client.Set(key, data, &ttl)
}

func (cs *CacheService) GetUser(userID int) (*User, error) {
	key := fmt.Sprintf("user:%d", userID)

	data, err := cs.client.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from cache: %w", err)
	}

	if data == nil {
		return nil, nil // User not found
	}

	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	return &user, nil
}

func (cs *CacheService) SetSession(token string, session *SessionData) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := fmt.Sprintf("session:%s", token)
	ttl := uint64(session.ExpiresAt - time.Now().Unix())

	return cs.client.Set(key, data, &ttl)
}

func (cs *CacheService) GetSession(token string) (*SessionData, error) {
	key := fmt.Sprintf("session:%s", token)

	data, err := cs.client.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get session from cache: %w", err)
	}

	if data == nil {
		return nil, nil // Session not found or expired
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &session, nil
}

func (cs *CacheService) InvalidateUserSessions(userID int) error {
	pattern := fmt.Sprintf("sessions:user:%d:*", userID)
	log.Printf("Would invalidate sessions matching pattern: %s", pattern)
	return nil
}

func (cs *CacheService) BulkSetUsers(users []*User) error {
	items := make(map[string][]byte)

	for _, user := range users {
		data, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("failed to marshal user %d: %w", user.ID, err)
		}

		key := fmt.Sprintf("user:%d", user.ID)
		items[key] = data
	}

	ttl := uint64(3600) // 1 hour
	return cs.client.BatchSet(items, &ttl)
}

func (cs *CacheService) BulkGetUsers(userIDs []int) (map[int]*User, error) {
	keys := make([]string, len(userIDs))
	for i, id := range userIDs {
		keys[i] = fmt.Sprintf("user:%d", id)
	}

	results, err := cs.client.BatchGet(keys)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk get users: %w", err)
	}

	users := make(map[int]*User)
	for key, data := range results {
		var userID int
		if _, err := fmt.Sscanf(key, "user:%d", &userID); err != nil {
			continue
		}

		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			log.Printf("Failed to unmarshal user %d: %v", userID, err)
			continue
		}

		users[userID] = &user
	}

	return users, nil
}

func (cs *CacheService) HealthCheck() error {
	// Test basic connectivity
	health, err := cs.client.Health()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	log.Printf("Cache server status: %s (uptime: %ds, connections: %d)",
		health.Status, health.UptimeSeconds, health.ActiveConnections)

	// Test basic operations
	testKey := "health_check_test"
	testValue := []byte("ping")

	if err := cs.client.Set(testKey, testValue, nil); err != nil {
		return fmt.Errorf("test set operation failed: %w", err)
	}

	value, err := cs.client.Get(testKey)
	if err != nil {
		return fmt.Errorf("test get operation failed: %w", err)
	}

	if string(value) != string(testValue) {
		return fmt.Errorf("test value mismatch: expected %s, got %s", testValue, value)
	}

	if _, err := cs.client.Delete(testKey); err != nil {
		return fmt.Errorf("test delete operation failed: %w", err)
	}

	log.Println("Health check passed")
	return nil
}

func runIntegrationDemo() {
	config := &subzero.ClientConfig{
		Address:           "127.0.0.1:8080",
		MaxConnections:    6,
		ConnectTimeout:    3 * time.Second,
		RequestTimeout:    75 * time.Millisecond,
		KeepAliveTime:     30 * time.Second,
		KeepAliveTimeout:  3 * time.Second,
		MaxRetries:        3,
		InitialBackoff:    5 * time.Millisecond,
		MaxBackoff:        500 * time.Millisecond,
		BackoffMultiplier: 2.0,
		EnablePooling:     true,
		PoolReuse:         true,
		EnableTLS:         false,
	}

	// Create cache service
	cacheService, err := NewCacheService(config)
	if err != nil {
		log.Fatalf("Failed to create cache service: %v", err)
	}
	defer cacheService.Close()

	// Demonstrate error handling with structured errors
	demoErrorHandling(cacheService)

	log.Println("=== Cache Service Integration Demo ===")

	// Health check
	if err := cacheService.HealthCheck(); err != nil {
		log.Printf("Health check failed: %v", err)
	}

	// Demo user operations
	demoUserOperations(cacheService)

	// Demo session operations
	demoSessionOperations(cacheService)

	// Demo batch operations
	demoBatchOperations(cacheService)

	log.Println("✅ Integration demo completed successfully")
}

func demoErrorHandling(cs *CacheService) {
	log.Println("=== Error Handling Demo ===")

	// Test with invalid key
	_, err := cs.GetUser(-1)
	if err != nil {
		if subzero.IsCacheError(err) {
			cacheErr := subzero.GetCacheError(err)
			log.Printf("Cache error detected: %s (code: %s, retryable: %t)",
				cacheErr.Message, cacheErr.Code, cacheErr.IsRetryable())
		} else {
			log.Printf("Non-cache error: %v", err)
		}
	}

	// Test timeout handling
	log.Println("Error handling demo completed")
}

func demoUserOperations(cs *CacheService) {
	log.Println("=== User Operations Demo ===")

	// Create test user
	user := &User{
		ID:       1001,
		Name:     "Alice Johnson",
		Email:    "alice@example.com",
		Age:      28,
		LastSeen: time.Now().Unix(),
	}

	// Store user
	if err := cs.SetUser(user); err != nil {
		log.Printf("Failed to store user: %v", err)
		return
	}
	log.Printf("✅ Stored user: %s", user.Name)

	// Retrieve user
	retrievedUser, err := cs.GetUser(user.ID)
	if err != nil {
		log.Printf("Failed to retrieve user: %v", err)
		return
	}

	if retrievedUser != nil {
		log.Printf("✅ Retrieved user: %s (age: %d)", retrievedUser.Name, retrievedUser.Age)
	} else {
		log.Println("❌ User not found")
	}

	// Test non-existent user
	missingUser, err := cs.GetUser(9999)
	if err != nil {
		log.Printf("Error retrieving missing user: %v", err)
	} else if missingUser == nil {
		log.Println("✅ Correctly handled missing user")
	}
}

func demoSessionOperations(cs *CacheService) {
	log.Println("=== Session Operations Demo ===")

	// Create test session
	session := &SessionData{
		UserID:    1001,
		Token:     "abc123xyz789",
		ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
		IP:        "192.168.1.100",
	}

	// Store session
	if err := cs.SetSession(session.Token, session); err != nil {
		log.Printf("Failed to store session: %v", err)
		return
	}
	log.Printf("✅ Stored session for user %d", session.UserID)

	// Retrieve session
	retrievedSession, err := cs.GetSession(session.Token)
	if err != nil {
		log.Printf("Failed to retrieve session: %v", err)
		return
	}

	if retrievedSession != nil {
		log.Printf("✅ Retrieved session for user %d from IP %s",
			retrievedSession.UserID, retrievedSession.IP)
	} else {
		log.Println("❌ Session not found")
	}
}

func demoBatchOperations(cs *CacheService) {
	log.Println("=== Batch Operations Demo ===")

	// Create multiple test users
	users := []*User{
		{ID: 2001, Name: "Bob Smith", Email: "bob@example.com", Age: 35, LastSeen: time.Now().Unix()},
		{ID: 2002, Name: "Carol Wilson", Email: "carol@example.com", Age: 42, LastSeen: time.Now().Unix()},
		{ID: 2003, Name: "David Brown", Email: "david@example.com", Age: 29, LastSeen: time.Now().Unix()},
	}

	// Bulk store users
	start := time.Now()
	if err := cs.BulkSetUsers(users); err != nil {
		log.Printf("Failed to bulk store users: %v", err)
		return
	}
	duration := time.Since(start)
	log.Printf("✅ Bulk stored %d users in %v", len(users), duration)

	// Bulk retrieve users
	userIDs := []int{2001, 2002, 2003, 9999} // (we also test with fake, to see if inconsistent)
	start = time.Now()
	retrievedUsers, err := cs.BulkGetUsers(userIDs)
	if err != nil {
		log.Printf("Failed to bulk retrieve users: %v", err)
		return
	}
	duration = time.Since(start)

	log.Printf("✅ Bulk retrieved %d/%d users in %v", len(retrievedUsers), len(userIDs), duration)
	for id, user := range retrievedUsers {
		log.Printf("   User %d: %s", id, user.Name)
	}
}
