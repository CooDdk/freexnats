package natsclient

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type StreamInfo struct {
	Name          string
	Subjects      []string
	Messages      uint64
	Bytes         uint64
	FirstSeq      uint64
	LastSeq       uint64
	ConsumerCount int
	Created       time.Time
	Description   string
	MaxMsgs       int64
	MaxBytes      int64
	MaxAge        time.Duration
	Storage       string
	Replicas      int
}

func (c *Client) ListStreams() ([]string, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	var streams []string
	for name := range c.js.StreamNames() {
		streams = append(streams, name)
	}

	return streams, nil
}

func (c *Client) GetStreamInfo(name string) (*StreamInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	info, err := c.js.StreamInfo(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	return convertStreamInfo(info), nil
}

func (c *Client) GetAllStreamInfos() ([]*StreamInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	var streams []*StreamInfo
	for info := range c.js.Streams() {
		streams = append(streams, convertStreamInfo(info))
	}

	return streams, nil
}

func (c *Client) CreateStream(name string, subjects []string, description string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	cfg := &nats.StreamConfig{
		Name:        name,
		Subjects:    subjects,
		Description: description,
		Storage:     nats.FileStorage,
		MaxMsgs:     -1,
		MaxBytes:    -1,
		MaxAge:      0,
		Replicas:    1,
		Retention:   nats.LimitsPolicy,
	}

	_, err := c.js.AddStream(cfg)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	return nil
}

// UpdateStream changes subjects and/or description on an existing stream,
// preserving every other config field (storage, retention, limits, replicas).
// Removing a subject only succeeds if no messages remain on that subject;
// otherwise JetStream rejects the update.
func (c *Client) UpdateStream(name string, subjects []string, description string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	info, err := c.js.StreamInfo(name)
	if err != nil {
		return fmt.Errorf("failed to fetch stream info: %w", err)
	}

	cfg := info.Config
	cfg.Subjects = subjects
	cfg.Description = description

	if _, err := c.js.UpdateStream(&cfg); err != nil {
		return fmt.Errorf("failed to update stream: %w", err)
	}
	return nil
}

func (c *Client) DeleteStream(name string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	err := c.js.DeleteStream(name)
	if err != nil {
		return fmt.Errorf("failed to delete stream: %w", err)
	}

	return nil
}

func (c *Client) PurgeStream(name string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	err := c.js.PurgeStream(name)
	if err != nil {
		return fmt.Errorf("failed to purge stream: %w", err)
	}

	return nil
}

func convertStreamInfo(info *nats.StreamInfo) *StreamInfo {
	storage := "file"
	if info.Config.Storage == nats.MemoryStorage {
		storage = "memory"
	}

	return &StreamInfo{
		Name:          info.Config.Name,
		Subjects:      info.Config.Subjects,
		Messages:      info.State.Msgs,
		Bytes:         info.State.Bytes,
		FirstSeq:      info.State.FirstSeq,
		LastSeq:       info.State.LastSeq,
		ConsumerCount: info.State.Consumers,
		Created:       info.Created,
		Description:   info.Config.Description,
		MaxMsgs:       info.Config.MaxMsgs,
		MaxBytes:      info.Config.MaxBytes,
		MaxAge:        info.Config.MaxAge,
		Storage:       storage,
		Replicas:      info.Config.Replicas,
	}
}
