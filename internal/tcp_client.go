package internal

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type TCPClientConfig struct {
	Address        string        `json:"address"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
	ReadTimeout    time.Duration `json:"read_timeout"`
	WriteTimeout   time.Duration `json:"write_timeout"`
	MaxRetries     int           `json:"max_retries"`
	RetryDelay     time.Duration `json:"retry_delay"`
	KeepAlive      bool          `json:"keep_alive"`
}

func DefaultTCPConfig() *TCPClientConfig {
	return &TCPClientConfig{
		Address:        "127.0.0.1:8081",
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    1 * time.Second,
		WriteTimeout:   1 * time.Second,
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		KeepAlive:      true,
	}
}

// TCP Protocol Client
type TCPClient struct {
	config *TCPClientConfig
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex
	closed int32
}

// Create new TCP client with given config
func NewTCPClient(config *TCPClientConfig) (*TCPClient, error) {
	if config == nil {
		config = DefaultTCPConfig()
	}

	if err := validateTCPConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := &TCPClient{
		config: config,
	}

	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return client, nil
}

// Validate TCP Config
func validateTCPConfig(config *TCPClientConfig) error {
	if config.Address == "" {
		return fmt.Errorf("address cannot be empty")
	}
	if config.ConnectTimeout <= 0 {
		return fmt.Errorf("connect_timeout must be positive")
	}
	if config.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}
	if config.WriteTimeout <= 0 {
		return fmt.Errorf("write_timeout must be positive")
	}
	if config.MaxRetries < 0 {
		return fmt.Errorf("max_retries cannot be negative")
	}
	return nil
}

// Establish connection to TCP port of subzero
func (c *TCPClient) connect() error {
	conn, err := net.DialTimeout("tcp", c.config.Address, c.config.ConnectTimeout)
	if err != nil {
		return fmt.Errorf("failed to dial %s: %w", c.config.Address, err)
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok && c.config.KeepAlive {
		if err := tcpConn.SetKeepAlive(true); err != nil {
			conn.Close()
			return fmt.Errorf("failed to set keep alive: %w", err)
		}
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)
	return nil
}

func (c *TCPClient) executeWithRetry(operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		c.mu.Lock()
		if atomic.LoadInt32(&c.closed) == 1 {
			c.mu.Unlock()
			return fmt.Errorf("client is closed")
		}

		err := operation()
		c.mu.Unlock()

		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !c.shouldRetryTCP(err) {
			return err
		}

		// Attempt to reconnect on connection errors
		if c.isConnectionError(err) {
			c.mu.Lock()
			c.reconnect()
			c.mu.Unlock()
		}

		// Don't sleep on the last attempt
		if attempt < c.config.MaxRetries {
			time.Sleep(c.config.RetryDelay)
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", c.config.MaxRetries, lastErr)
}

func (c *TCPClient) shouldRetryTCP(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Retry on connection errors, timeouts, and temporary errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary") {
		return true
	}

	if netErr, ok := err.(net.Error); ok {
		return netErr.Temporary() || netErr.Timeout()
	}

	return false
}

func (c *TCPClient) isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "EOF")
}

func (c *TCPClient) reconnect() {
	if c.conn != nil {
		c.conn.Close()
	}
	c.connect() // Ignore error, will be caught on next operation
}

func (c *TCPClient) sendCommand(command string) (string, error) {
	// Set write timeout
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout)); err != nil {
		return "", fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Write command
	if _, err := c.writer.WriteString(command + "\n"); err != nil {
		return "", fmt.Errorf("failed to write command: %w", err)
	}

	if err := c.writer.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush command: %w", err)
	}

	// Set read timeout
	if err := c.conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout)); err != nil {
		return "", fmt.Errorf("failed to set read deadline: %w", err)
	}

	// Read response
	response, err := c.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return strings.TrimSpace(response), nil
}

func (c *TCPClient) Set(key, value string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	return c.executeWithRetry(func() error {
		command := fmt.Sprintf("SET %s %s", key, value)
		response, err := c.sendCommand(command)
		if err != nil {
			return err
		}

		if response != "STORED" {
			if strings.HasPrefix(response, "ERROR") {
				return fmt.Errorf("set failed: %s", response)
			}
			return fmt.Errorf("unexpected response: %s", response)
		}

		return nil
	})
}

func (c *TCPClient) Get(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}

	var result string
	err := c.executeWithRetry(func() error {
		command := fmt.Sprintf("GET %s", key)
		response, err := c.sendCommand(command)
		if err != nil {
			return err
		}

		if response == "NOT_FOUND" {
			result = ""
			return nil
		}

		if strings.HasPrefix(response, "VALUE ") {
			result = response[6:] // Remove "VALUE " prefix
			return nil
		}

		if strings.HasPrefix(response, "ERROR") {
			return fmt.Errorf("get failed: %s", response)
		}

		result = response
		return nil
	})

	return result, err
}

func (c *TCPClient) Delete(key string) (bool, error) {
	if key == "" {
		return false, fmt.Errorf("key cannot be empty")
	}

	var found bool
	err := c.executeWithRetry(func() error {
		command := fmt.Sprintf("DEL %s", key)
		response, err := c.sendCommand(command)
		if err != nil {
			return err
		}

		switch response {
		case "DELETED":
			found = true
			return nil
		case "NOT_FOUND":
			found = false
			return nil
		default:
			if strings.HasPrefix(response, "ERROR") {
				return fmt.Errorf("delete failed: %s", response)
			}
			return fmt.Errorf("unexpected response: %s", response)
		}
	})

	return found, err
}

func (c *TCPClient) Ping() error {
	return c.executeWithRetry(func() error {
		response, err := c.sendCommand("PING")
		if err != nil {
			return err
		}

		if response != "PONG" {
			return fmt.Errorf("unexpected ping response: %s", response)
		}

		return nil
	})
}

// Close closes the TCP connection
func (c *TCPClient) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil // Already closed
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		// Send quit command if possible
		c.sendCommand("QUIT")
		return c.conn.Close()
	}

	return nil
}

func (c *TCPClient) IsConnected() bool {
	if atomic.LoadInt32(&c.closed) == 1 {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn != nil
}

func (c *TCPClient) GetConfig() TCPClientConfig {
	return *c.config
}
