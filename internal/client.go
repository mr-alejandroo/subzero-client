package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	pb "github.com/mr.alejandroo/subzero-client/proto"
)

type ClientConfig struct {
	// Connection settings
	Address          string        `json:"address"`
	MaxConnections   int           `json:"max_connections"`
	ConnectTimeout   time.Duration `json:"connect_timeout"`
	RequestTimeout   time.Duration `json:"request_timeout"`
	KeepAliveTime    time.Duration `json:"keep_alive_time"`
	KeepAliveTimeout time.Duration `json:"keep_alive_timeout"`

	// Retry settings
	MaxRetries        int           `json:"max_retries"`
	InitialBackoff    time.Duration `json:"initial_backoff"`
	MaxBackoff        time.Duration `json:"max_backoff"`
	BackoffMultiplier float64       `json:"backoff_multiplier"`

	// Connection pool settings
	EnablePooling bool `json:"enable_pooling"`
	PoolReuse     bool `json:"pool_reuse"`

	// TLS settings
	EnableTLS     bool   `json:"enable_tls"`
	TLSServerName string `json:"tls_server_name"`
}

func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		Address:           "127.0.0.1:8080",
		MaxConnections:    4,
		ConnectTimeout:    5 * time.Second,
		RequestTimeout:    100 * time.Millisecond,
		KeepAliveTime:     10 * time.Second,
		KeepAliveTimeout:  time.Second,
		MaxRetries:        3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        1 * time.Second,
		BackoffMultiplier: 2.0,
		EnablePooling:     true,
		PoolReuse:         true,
		EnableTLS:         false,
	}
}

func ProductionConfig(address string) *ClientConfig {
	config := DefaultConfig()
	config.Address = address
	config.MaxConnections = 8
	config.RequestTimeout = 500 * time.Millisecond
	config.EnableTLS = true
	return config
}

// Subzero Client
type Client struct {
	config      *ClientConfig
	clients     []pb.CacheServiceClient
	connections []*grpc.ClientConn
	contextPool sync.Pool
	counter     int64
	mu          sync.RWMutex
	closed      int32
}

type contextWrapper struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := &Client{
		config:      config,
		clients:     make([]pb.CacheServiceClient, config.MaxConnections),
		connections: make([]*grpc.ClientConn, config.MaxConnections),
	}

	// Context Pool
	client.contextPool.New = func() interface{} {
		ctx, cancel := context.WithTimeout(context.Background(), config.RequestTimeout)
		return &contextWrapper{ctx: ctx, cancel: cancel}
	}

	// Create connections
	if err := client.initConnections(); err != nil {
		return nil, fmt.Errorf("failed to initialize connections: %w", err)
	}

	return client, nil
}

// validateConfig validates the client configuration
func validateConfig(config *ClientConfig) error {
	if config.Address == "" {
		return fmt.Errorf("address cannot be empty")
	}
	if config.MaxConnections < 1 {
		return fmt.Errorf("max_connections must be at least 1")
	}
	if config.MaxConnections > 100 {
		return fmt.Errorf("max_connections cannot exceed 100")
	}
	if config.ConnectTimeout <= 0 {
		return fmt.Errorf("connect_timeout must be positive")
	}
	if config.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be positive")
	}
	if config.MaxRetries < 0 {
		return fmt.Errorf("max_retries cannot be negative")
	}
	if config.BackoffMultiplier <= 1.0 {
		return fmt.Errorf("backoff_multiplier must be greater than 1.0")
	}
	return nil
}

// initConnections establishes gRPC connections to the server
func (c *Client) initConnections() error {
	opts := c.buildDialOptions()

	for i := 0; i < c.config.MaxConnections; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), c.config.ConnectTimeout)
		conn, err := grpc.DialContext(ctx, c.config.Address, opts...)
		cancel()

		if err != nil {
			// Close existing connections on failure
			for j := 0; j < i; j++ {
				c.connections[j].Close()
			}
			return fmt.Errorf("failed to connect to %s: %w", c.config.Address, err)
		}

		c.connections[i] = conn
		c.clients[i] = pb.NewCacheServiceClient(conn)
	}

	return nil
}

// Create gRPC Dial
func (c *Client) buildDialOptions() []grpc.DialOption {
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                c.config.KeepAliveTime,
			Timeout:             c.config.KeepAliveTimeout,
			PermitWithoutStream: true,
		}),
		grpc.WithInitialWindowSize(1 << 20),     // 1MB
		grpc.WithInitialConnWindowSize(1 << 20), // 1MB
	}

	if c.config.EnableTLS {
		// TODO: add tls cred support here :0
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return opts
}

// getClient returns the next available client using round-robin
func (c *Client) getClient() (pb.CacheServiceClient, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, fmt.Errorf("client is closed")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.clients) == 0 {
		return nil, fmt.Errorf("no clients available")
	}

	index := atomic.AddInt64(&c.counter, 1) % int64(len(c.clients))
	return c.clients[index], nil
}

// getContext gets a context from the pool or creates a new one
func (c *Client) getContext() *contextWrapper {
	if c.config.PoolReuse {
		return c.contextPool.Get().(*contextWrapper)
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.config.RequestTimeout)
	return &contextWrapper{ctx: ctx, cancel: cancel}
}

// putContext returns a context to the pool
func (c *Client) putContext(wrapper *contextWrapper) {
	wrapper.cancel()
	if c.config.PoolReuse {
		// Reset context for reuse
		ctx, cancel := context.WithTimeout(context.Background(), c.config.RequestTimeout)
		wrapper.ctx = ctx
		wrapper.cancel = cancel
		c.contextPool.Put(wrapper)
	}
}

// executeWithRetry executes a function with retry logic
func (c *Client) executeWithRetry(operation func(context.Context, pb.CacheServiceClient) error) error {
	var lastErr error
	backoff := c.config.InitialBackoff

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		client, err := c.getClient()
		if err != nil {
			return fmt.Errorf("failed to get client: %w", err)
		}

		ctxWrapper := c.getContext()
		err = operation(ctxWrapper.ctx, client)
		c.putContext(ctxWrapper)

		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}

		// Don't sleep on the last attempt
		if attempt < c.config.MaxRetries {
			time.Sleep(backoff)
			backoff = time.Duration(float64(backoff) * c.config.BackoffMultiplier)
			if backoff > c.config.MaxBackoff {
				backoff = c.config.MaxBackoff
			}
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", c.config.MaxRetries, lastErr)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if grpcErr, ok := status.FromError(err); ok {
		switch grpcErr.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
			return true
		case codes.InvalidArgument, codes.NotFound, codes.PermissionDenied:
			return false
		}
	}
	return true
}

// Set stores a key-value pair in the cache
func (c *Client) Set(key string, value []byte, ttlSeconds *uint64) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	return c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		req := &pb.SetRequest{
			Key:   key,
			Value: value,
		}
		if ttlSeconds != nil {
			req.TtlSeconds = ttlSeconds
		}

		resp, err := client.Set(ctx, req)
		if err != nil {
			return fmt.Errorf("set operation failed: %w", err)
		}

		if !resp.Success {
			return fmt.Errorf("set failed: %s", resp.GetError())
		}

		return nil
	})
}

// Get retrieves a value by key from the cache
func (c *Client) Get(key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	var result []byte
	err := c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		req := &pb.GetRequest{
			Key: key,
		}

		resp, err := client.Get(ctx, req)
		if err != nil {
			return fmt.Errorf("get operation failed: %w", err)
		}

		if !resp.Found {
			result = nil
			return nil
		}

		result = resp.GetValue()
		return nil
	})

	return result, err
}

// GetWithMetadata retrieves a value and its metadata by key
func (c *Client) GetWithMetadata(key string) ([]byte, *pb.EntryMetadata, error) {
	if key == "" {
		return nil, nil, fmt.Errorf("key cannot be empty")
	}

	var result []byte
	var metadata *pb.EntryMetadata

	err := c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		includeMetadata := true
		req := &pb.GetRequest{
			Key:             key,
			IncludeMetadata: &includeMetadata,
		}

		resp, err := client.Get(ctx, req)
		if err != nil {
			return fmt.Errorf("get operation failed: %w", err)
		}

		if !resp.Found {
			result = nil
			metadata = nil
			return nil
		}

		result = resp.GetValue()
		metadata = resp.GetMetadata()
		return nil
	})

	return result, metadata, err
}

// Delete removes a key from the cache
func (c *Client) Delete(key string) (bool, error) {
	if key == "" {
		return false, fmt.Errorf("key cannot be empty")
	}

	var found bool
	err := c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		req := &pb.DeleteRequest{
			Key: key,
		}

		resp, err := client.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("delete operation failed: %w", err)
		}

		if !resp.Success {
			return fmt.Errorf("delete operation was not successful")
		}

		found = resp.Found
		return nil
	})

	return found, err
}

// BatchGet retrieves multiple keys in a single operation
func (c *Client) BatchGet(keys []string) (map[string][]byte, error) {
	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	for _, key := range keys {
		if key == "" {
			return nil, fmt.Errorf("keys cannot contain empty strings")
		}
	}

	var results map[string][]byte
	err := c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		req := &pb.BatchGetRequest{
			Keys: keys,
		}

		resp, err := client.BatchGet(ctx, req)
		if err != nil {
			return fmt.Errorf("batch get operation failed: %w", err)
		}

		results = make(map[string][]byte)
		for key, getResp := range resp.Results {
			if getResp.Found {
				results[key] = getResp.GetValue()
			}
		}

		return nil
	})

	return results, err
}

// BatchSet stores multiple key-value pairs in a single operation
func (c *Client) BatchSet(items map[string][]byte, ttlSeconds *uint64) error {
	if len(items) == 0 {
		return nil
	}

	for key, value := range items {
		if key == "" {
			return fmt.Errorf("keys cannot be empty")
		}
		if value == nil {
			return fmt.Errorf("values cannot be nil for key: %s", key)
		}
	}

	return c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		var requests []*pb.SetRequest
		for key, value := range items {
			req := &pb.SetRequest{
				Key:   key,
				Value: value,
			}
			if ttlSeconds != nil {
				req.TtlSeconds = ttlSeconds
			}
			requests = append(requests, req)
		}

		req := &pb.BatchSetRequest{
			Requests: requests,
		}

		resp, err := client.BatchSet(ctx, req)
		if err != nil {
			return fmt.Errorf("batch set operation failed: %w", err)
		}

		// Check all responses for errors
		for i, setResp := range resp.Responses {
			if !setResp.Success {
				return fmt.Errorf("batch set item %d failed: %s", i, setResp.GetError())
			}
		}

		return nil
	})
}

// Health checks the health of the cache server
func (c *Client) Health() (*pb.HealthResponse, error) {
	var health *pb.HealthResponse
	err := c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		req := &pb.HealthRequest{}
		resp, err := client.Health(ctx, req)
		if err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}
		health = resp
		return nil
	})

	return health, err
}

// Retrive server metrics (Prometheus format)
func (c *Client) Metrics() (string, error) {
	var metrics string
	err := c.executeWithRetry(func(ctx context.Context, client pb.CacheServiceClient) error {
		req := &pb.MetricsRequest{}
		resp, err := client.Metrics(ctx, req)
		if err != nil {
			return fmt.Errorf("metrics request failed: %w", err)
		}
		metrics = resp.PrometheusMetrics
		return nil
	})

	return metrics, err
}

// Close closes all connections and cleans up resources
func (c *Client) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil // Already closed
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for i, conn := range c.connections {
		if conn != nil {
			if err := conn.Close(); err != nil {
				lastErr = fmt.Errorf("failed to close connection %d: %w", i, err)
			}
		}
	}

	return lastErr
}

// IsConnected checks if the client has active connections
func (c *Client) IsConnected() bool {
	if atomic.LoadInt32(&c.closed) == 1 {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.connections) > 0
}

// GetConfig returns a copy of the client configuration
func (c *Client) GetConfig() ClientConfig {
	return *c.config
}
