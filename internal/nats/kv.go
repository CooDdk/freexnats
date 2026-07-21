package natsclient

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type BucketInfo struct {
	Name        string
	Description string
	Values      int
	History     int64
	TTL         time.Duration
	Replicas    int
	StreamName  string
	Bytes       uint64
	Created     time.Time
}

type KVEntry struct {
	Key       string
	Value     string
	Revision  uint64
	Created   time.Time
	IsDeleted bool
}

func (c *Client) ListKVBuckets() ([]*BucketInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	var buckets []*BucketInfo
	for name := range c.js.StreamNames() {
		if !isKVStream(name) {
			continue
		}
		bucketName := name[3:] // strip "KV_"
		info, err := c.getBucketInfo(bucketName)
		if err != nil {
			continue
		}
		buckets = append(buckets, info)
	}

	return buckets, nil
}

func (c *Client) GetKVBucketInfo(name string) (*BucketInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}
	return c.getBucketInfo(name)
}

func (c *Client) getBucketInfo(name string) (*BucketInfo, error) {
	kv, err := c.js.KeyValue(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open KV bucket: %w", err)
	}

	status, err := kv.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get KV status: %w", err)
	}

	kvs := status.(*nats.KeyValueBucketStatus)
	streamInfo := kvs.StreamInfo()

	desc := ""
	if streamInfo != nil {
		desc = streamInfo.Config.Description
	}

	return &BucketInfo{
		Name:        name,
		Description: desc,
		Values:      int(kvs.Values()),
		History:     kvs.History(),
		TTL:         kvs.TTL(),
		Replicas:    func() int { if streamInfo != nil { return streamInfo.Config.Replicas }; return 1 }(),
		StreamName:  kvs.BackingStore(),
		Bytes:       kvs.Bytes(),
		Created:     func() time.Time { if streamInfo != nil { return streamInfo.Created }; return time.Time{} }(),
	}, nil
}

func (c *Client) CreateKVBucket(name, description string, history int64, ttl time.Duration, replicas int) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	cfg := nats.KeyValueConfig{
		Bucket:      name,
		Description: description,
		History:     uint8(history),
		TTL:         ttl,
		Replicas:    replicas,
		Storage:     nats.FileStorage,
	}

	_, err := c.js.CreateKeyValue(&cfg)
	if err != nil {
		return fmt.Errorf("failed to create KV bucket: %w", err)
	}

	return nil
}

func (c *Client) DeleteKVBucket(name string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	err := c.js.DeleteKeyValue(name)
	if err != nil {
		return fmt.Errorf("failed to delete KV bucket: %w", err)
	}

	return nil
}

func (c *Client) ListKVKeys(bucket string) ([]string, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	kv, err := c.js.KeyValue(bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to open KV bucket: %w", err)
	}

	keys, err := kv.Keys()
	if err != nil {
		if err == nats.ErrNoKeysFound {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	return keys, nil
}

func (c *Client) GetKVValue(bucket, key string) (*KVEntry, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	kv, err := c.js.KeyValue(bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to open KV bucket: %w", err)
	}

	entry, err := kv.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	return &KVEntry{
		Key:      entry.Key(),
		Value:    string(entry.Value()),
		Revision: entry.Revision(),
		Created:  entry.Created(),
	}, nil
}

func (c *Client) PutKVValue(bucket, key, value string) (uint64, error) {
	if !c.IsConnected() {
		return 0, fmt.Errorf("not connected to NATS")
	}

	kv, err := c.js.KeyValue(bucket)
	if err != nil {
		return 0, fmt.Errorf("failed to open KV bucket: %w", err)
	}

	rev, err := kv.Put(key, []byte(value))
	if err != nil {
		return 0, fmt.Errorf("failed to put value: %w", err)
	}

	return rev, nil
}

func (c *Client) DeleteKVKey(bucket, key string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	kv, err := c.js.KeyValue(bucket)
	if err != nil {
		return fmt.Errorf("failed to open KV bucket: %w", err)
	}

	err = kv.Delete(key)
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}

	return nil
}

func (c *Client) GetKVHistory(bucket, key string) ([]*KVEntry, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}

	kv, err := c.js.KeyValue(bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to open KV bucket: %w", err)
	}

	history, err := kv.History(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	var entries []*KVEntry
	for _, e := range history {
		entries = append(entries, &KVEntry{
			Key:       e.Key(),
			Value:     string(e.Value()),
			Revision:  e.Revision(),
			Created:   e.Created(),
			IsDeleted: e.Operation() == nats.KeyValueDelete,
		})
	}

	return entries, nil
}

func isKVStream(name string) bool {
	return len(name) > 3 && name[:3] == "KV_"
}
