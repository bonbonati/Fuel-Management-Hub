// Package kv is a minimal key-value abstraction over Valkey (Redis-compatible).
//
// Services use it for short-lived, ephemeral state that must be shared by all
// replicas but never needs to be durable: rotating QR pass tokens (30s TTL),
// station scan sessions (minutes), and login throttling counters. Keeping it
// behind this small interface lets unit tests run against the in-memory
// implementation without a Valkey server.
package kv

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

// Store is the subset of key-value operations the services need.
type Store interface {
	// SetJSON stores v (JSON encoded) under key for ttl. When onlyIfAbsent is
	// true the write happens only if the key does not exist; ok reports
	// whether the value was written.
	SetJSON(ctx context.Context, key string, v any, ttl time.Duration, onlyIfAbsent bool) (ok bool, err error)
	// GetJSON loads key into dst; found is false when the key is missing/expired.
	GetJSON(ctx context.Context, key string, dst any) (found bool, err error)
	// Delete removes key (no error if missing).
	Delete(ctx context.Context, key string) error
	// Incr increments the counter at key, setting ttl when it is created,
	// and returns the new value. Used for fixed-window rate limiting.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	// Ping checks connectivity (readiness probe).
	Ping(ctx context.Context) error
}

// Valkey implements Store with the official valkey-go client.
type Valkey struct{ c valkey.Client }

// NewValkey connects to the Valkey server at addr ("host:port").
func NewValkey(addr, password string) (*Valkey, error) {
	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{addr},
		Password:     password,
		DisableCache: true, // we never read the same key twice in a request; skip client-side caching
	})
	if err != nil {
		return nil, err
	}
	return &Valkey{c: c}, nil
}

// Close releases the client's connections.
func (v *Valkey) Close() { v.c.Close() }

func (v *Valkey) SetJSON(ctx context.Context, key string, val any, ttl time.Duration, onlyIfAbsent bool) (bool, error) {
	b, err := json.Marshal(val)
	if err != nil {
		return false, err
	}
	cmd := v.c.B().Set().Key(key).Value(string(b))
	var res valkey.ValkeyResult
	if onlyIfAbsent {
		res = v.c.Do(ctx, cmd.Nx().Px(ttl).Build())
	} else {
		res = v.c.Do(ctx, cmd.Px(ttl).Build())
	}
	if err := res.Error(); err != nil {
		if valkey.IsValkeyNil(err) { // NX lost the race: key exists
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (v *Valkey) GetJSON(ctx context.Context, key string, dst any) (bool, error) {
	s, err := v.c.Do(ctx, v.c.B().Get().Key(key).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(s), dst)
}

func (v *Valkey) Delete(ctx context.Context, key string) error {
	return v.c.Do(ctx, v.c.B().Del().Key(key).Build()).Error()
}

func (v *Valkey) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := v.c.Do(ctx, v.c.B().Incr().Key(key).Build()).AsInt64()
	if err != nil {
		return 0, err
	}
	if n == 1 { // first hit in this window: start the window
		if err := v.c.Do(ctx, v.c.B().Pexpire().Key(key).Milliseconds(ttl.Milliseconds()).Build()).Error(); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (v *Valkey) Ping(ctx context.Context) error {
	return v.c.Do(ctx, v.c.B().Ping().Build()).Error()
}

// Memory is an in-process Store for unit tests. Expiry uses the injectable
// Now clock so tests can fast-forward time.
type Memory struct {
	mu   sync.Mutex
	data map[string]memItem
	Now  func() time.Time
}

type memItem struct {
	val     []byte
	counter int64
	expires time.Time
}

// NewMemory returns an empty in-memory store using the real clock.
func NewMemory() *Memory { return &Memory{data: map[string]memItem{}, Now: time.Now} }

func (m *Memory) live(key string) (memItem, bool) {
	it, ok := m.data[key]
	if ok && !it.expires.IsZero() && !m.Now().Before(it.expires) {
		delete(m.data, key)
		return memItem{}, false
	}
	return it, ok
}

func (m *Memory) SetJSON(_ context.Context, key string, v any, ttl time.Duration, onlyIfAbsent bool) (bool, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.live(key); exists && onlyIfAbsent {
		return false, nil
	}
	m.data[key] = memItem{val: b, expires: m.Now().Add(ttl)}
	return true, nil
}

func (m *Memory) GetJSON(_ context.Context, key string, dst any) (bool, error) {
	m.mu.Lock()
	it, ok := m.live(key)
	m.mu.Unlock()
	if !ok {
		return false, nil
	}
	if it.val == nil {
		return false, errors.New("kv: key holds a counter, not JSON")
	}
	return true, json.Unmarshal(it.val, dst)
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *Memory) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.live(key)
	if !ok {
		it = memItem{expires: m.Now().Add(ttl)}
	}
	it.counter++
	m.data[key] = it
	return it.counter, nil
}

func (m *Memory) Ping(context.Context) error { return nil }
