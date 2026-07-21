package natsclient

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Config struct {
	URL          string
	Username     string
	Password     string
	Token        string
	Timeout      time.Duration
	MaxReconnect int
}

func DefaultConfig() *Config {
	return &Config{
		URL:          nats.DefaultURL,
		Timeout:      10 * time.Second,
		MaxReconnect: 10,
	}
}

type Client struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	config *Config
}

func NewClient(config *Config) *Client {
	if config == nil {
		config = DefaultConfig()
	}
	return &Client{
		config: config,
	}
}

func (c *Client) Connect() error {
	opts := []nats.Option{
		nats.Timeout(c.config.Timeout),
		nats.MaxReconnects(c.config.MaxReconnect),
		nats.ReconnectWait(2 * time.Second),
	}

	if c.config.Username != "" {
		opts = append(opts, nats.UserInfo(c.config.Username, c.config.Password))
	}
	if c.config.Token != "" {
		opts = append(opts, nats.Token(c.config.Token))
	}

	nc, err := nats.Connect(c.config.URL, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	c.nc = nc
	c.js = js
	return nil
}

func (c *Client) Disconnect() {
	if c.nc != nil {
		c.nc.Close()
		c.nc = nil
		c.js = nil
	}
}

func (c *Client) IsConnected() bool {
	return c.nc != nil && c.nc.IsConnected()
}

func (c *Client) ServerURL() string {
	if c.nc != nil {
		return c.nc.ConnectedUrl()
	}
	return c.config.URL
}

func (c *Client) ServerName() string {
	if c.nc != nil {
		return c.nc.ConnectedServerName()
	}
	return ""
}

func (c *Client) ServerVersion() string {
	if c.nc != nil {
		return c.nc.ConnectedServerVersion()
	}
	return ""
}

func (c *Client) Conn() *nats.Conn {
	return c.nc
}

func (c *Client) JS() nats.JetStreamContext {
	return c.js
}
