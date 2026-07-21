package natsclient

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type ConsumerInfo struct {
	Name           string
	StreamName     string
	Description    string
	Delivered      uint64
	AckFloorStream uint64
	AckPending     int
	Pending        uint64
	NumPending     uint64
	NumAckPending  int
	NumRedelivered int
	Created        time.Time
	AckPolicy      string
	DeliverPolicy  string
	DurableName    string
	FilterSubject  string
	MaxDeliver     int
	ReplayPolicy   string
}

func (c *Client) ListConsumers(streamName string) ([]string, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	var consumers []string
	for name := range c.js.ConsumerNames(streamName) {
		consumers = append(consumers, name)
	}

	return consumers, nil
}

func (c *Client) GetConsumerInfo(streamName, consumerName string) (*ConsumerInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	info, err := c.js.ConsumerInfo(streamName, consumerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer info: %w", err)
	}

	return convertConsumerInfo(info), nil
}

func (c *Client) GetAllConsumerInfos(streamName string) ([]*ConsumerInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	var consumers []*ConsumerInfo
	for info := range c.js.Consumers(streamName) {
		consumers = append(consumers, convertConsumerInfo(info))
	}

	return consumers, nil
}

func (c *Client) CreateConsumer(streamName, consumerName, filterSubject, durableName string, description string, ackPolicy, deliverPolicy string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	ack := nats.AckExplicitPolicy
	switch ackPolicy {
	case "all":
		ack = nats.AckAllPolicy
	case "none":
		ack = nats.AckNonePolicy
	}

	deliver := nats.DeliverAllPolicy
	switch deliverPolicy {
	case "last":
		deliver = nats.DeliverLastPolicy
	case "new":
		deliver = nats.DeliverNewPolicy
	}

	cfg := &nats.ConsumerConfig{
		Name:          consumerName,
		Durable:       durableName,
		Description:   description,
		FilterSubject: filterSubject,
		AckPolicy:     ack,
		DeliverPolicy: deliver,
		MaxDeliver:    -1,
		ReplayPolicy:  nats.ReplayInstantPolicy,
		AckWait:       30 * time.Second,
	}

	_, err := c.js.AddConsumer(streamName, cfg)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	return nil
}

func (c *Client) DeleteConsumer(streamName, consumerName string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	err := c.js.DeleteConsumer(streamName, consumerName)
	if err != nil {
		return fmt.Errorf("failed to delete consumer: %w", err)
	}

	return nil
}

// ResetConsumer rewinds a consumer's cursor by deleting and recreating it
// with its original configuration. JetStream has no first-class "reset
// cursor" operation, so this is the idiomatic path: preserve every config
// field (durable, filter, ack/deliver policy, etc.) and let the deliver
// policy dictate where playback resumes — DeliverAll restarts from message
// 1, DeliverNew from the next incoming message.
func (c *Client) ResetConsumer(streamName, consumerName string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	info, err := c.js.ConsumerInfo(streamName, consumerName)
	if err != nil {
		return fmt.Errorf("failed to read consumer config: %w", err)
	}
	cfg := info.Config

	if err := c.js.DeleteConsumer(streamName, consumerName); err != nil {
		return fmt.Errorf("failed to delete consumer for reset: %w", err)
	}
	if _, err := c.js.AddConsumer(streamName, &cfg); err != nil {
		return fmt.Errorf("failed to recreate consumer after delete: %w", err)
	}
	return nil
}

func convertConsumerInfo(info *nats.ConsumerInfo) *ConsumerInfo {
	ackPolicy := "explicit"
	switch info.Config.AckPolicy {
	case nats.AckNonePolicy:
		ackPolicy = "none"
	case nats.AckAllPolicy:
		ackPolicy = "all"
	}

	deliverPolicy := "all"
	switch info.Config.DeliverPolicy {
	case nats.DeliverLastPolicy:
		deliverPolicy = "last"
	case nats.DeliverNewPolicy:
		deliverPolicy = "new"
	}

	replayPolicy := "instant"
	if info.Config.ReplayPolicy == nats.ReplayOriginalPolicy {
		replayPolicy = "original"
	}

	return &ConsumerInfo{
		Name:           info.Name,
		StreamName:     info.Stream,
		Description:    info.Config.Description,
		Delivered:      info.Delivered.Stream,
		AckFloorStream: info.AckFloor.Stream,
		AckPending:     info.NumAckPending,
		Pending:        info.NumPending,
		NumPending:     info.NumPending,
		NumAckPending:  info.NumAckPending,
		NumRedelivered: info.NumRedelivered,
		Created:        info.Created,
		AckPolicy:      ackPolicy,
		DeliverPolicy:  deliverPolicy,
		DurableName:    info.Config.Durable,
		FilterSubject:  info.Config.FilterSubject,
		MaxDeliver:     info.Config.MaxDeliver,
		ReplayPolicy:   replayPolicy,
	}
}
