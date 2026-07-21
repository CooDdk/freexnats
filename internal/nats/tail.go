package natsclient

import (
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// TailMsg is a single live message captured by a TailSub. Data is copied out
// of the nats.Msg so callers can hold on to it after the callback returns.
type TailMsg struct {
	Subject  string
	Data     []byte
	Received time.Time
	Sequence uint64 // JetStream stream seq if available, 0 otherwise
}

// TailSub subscribes to one or more subject filters on the core NATS connection
// and keeps a bounded ring of the most recent messages. It is safe to call
// Snapshot/Count/Clear concurrently with incoming callbacks.
type TailSub struct {
	nc   *nats.Conn
	subs []*nats.Subscription

	mu    sync.Mutex
	buf   []TailMsg
	cap   int
	total uint64
}

// NewTailSub subscribes to each of the given subjects using core NATS. Duplicate
// subjects are skipped. If any subscription fails the partial state is torn
// down and an error is returned.
func (c *Client) NewTailSub(subjects []string, capacity int) (*TailSub, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to NATS")
	}
	if capacity <= 0 {
		capacity = 500
	}
	ts := &TailSub{nc: c.nc, cap: capacity}
	seen := map[string]bool{}
	for _, subj := range subjects {
		if subj == "" || seen[subj] {
			continue
		}
		seen[subj] = true
		sub, err := c.nc.Subscribe(subj, ts.onMsg)
		if err != nil {
			ts.Stop()
			return nil, fmt.Errorf("subscribe %q: %w", subj, err)
		}
		ts.subs = append(ts.subs, sub)
	}
	if len(ts.subs) == 0 {
		return nil, fmt.Errorf("no subjects to subscribe to")
	}
	return ts, nil
}

func (t *TailSub) onMsg(m *nats.Msg) {
	msg := TailMsg{
		Subject:  m.Subject,
		Data:     append([]byte(nil), m.Data...),
		Received: time.Now(),
	}
	if meta, err := m.Metadata(); err == nil && meta != nil {
		msg.Sequence = meta.Sequence.Stream
	}
	t.mu.Lock()
	t.buf = append(t.buf, msg)
	if len(t.buf) > t.cap {
		t.buf = t.buf[len(t.buf)-t.cap:]
	}
	t.total++
	t.mu.Unlock()
}

// Snapshot returns a copy of the current buffer (oldest first).
func (t *TailSub) Snapshot() []TailMsg {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TailMsg, len(t.buf))
	copy(out, t.buf)
	return out
}

// Total returns the cumulative message count since Start (survives Clear).
func (t *TailSub) Total() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.total
}

// Clear drops all buffered messages but preserves the running total.
func (t *TailSub) Clear() {
	t.mu.Lock()
	t.buf = nil
	t.mu.Unlock()
}

// Capacity returns the max buffer size.
func (t *TailSub) Capacity() int { return t.cap }

// Stop unsubscribes from all subjects. Safe to call multiple times.
func (t *TailSub) Stop() {
	for _, s := range t.subs {
		_ = s.Unsubscribe()
	}
	t.subs = nil
}
