package natsclient

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

type StoredMsg struct {
	Subject   string
	Sequence  uint64
	Data      string
	Timestamp string
	Headers   map[string][]string
}

func (c *Client) GetStreamMessage(stream string, seq uint64) (*StoredMsg, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	msg, err := c.js.GetMsg(stream, seq)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	return convertStoredMsg(msg), nil
}

func (c *Client) GetLastMessage(stream string) (*StoredMsg, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	msg, err := c.js.GetLastMsg(stream, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get last message: %w", err)
	}

	return convertStoredMsg(msg), nil
}

func (c *Client) GetMessageBySubject(stream, subject string) (*StoredMsg, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	msg, err := c.js.GetLastMsg(stream, subject)
	if err != nil {
		return nil, fmt.Errorf("failed to get message by subject: %w", err)
	}

	return convertStoredMsg(msg), nil
}

func (c *Client) PublishMessage(subject string, data []byte, headers map[string][]string) (uint64, error) {
	if !c.IsConnected() {
		return 0, fmt.Errorf("not connected to NATS")
	}

	if len(headers) == 0 {
		pub, err := c.js.Publish(subject, data)
		if err != nil {
			return 0, fmt.Errorf("failed to publish message: %w", err)
		}
		return pub.Sequence, nil
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}
	for k, vs := range headers {
		for _, v := range vs {
			msg.Header.Add(k, v)
		}
	}
	pub, err := c.js.PublishMsg(msg)
	if err != nil {
		return 0, fmt.Errorf("failed to publish message: %w", err)
	}
	return pub.Sequence, nil
}

func (c *Client) DeleteStreamMessage(stream string, seq uint64) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	err := c.js.DeleteMsg(stream, seq)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil
}

func convertStoredMsg(msg *nats.RawStreamMsg) *StoredMsg {
	headers := make(map[string][]string)
	if msg.Header != nil {
		for k, v := range msg.Header {
			headers[k] = v
		}
	}

	return &StoredMsg{
		Subject:   msg.Subject,
		Sequence:  msg.Sequence,
		Data:      string(msg.Data),
		Timestamp: msg.Time.Format("2006-01-02 15:04:05"),
		Headers:   headers,
	}
}
