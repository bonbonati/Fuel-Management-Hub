package kv

import (
	"context"
	"testing"
	"time"
)

// The Memory store is the test double for every service, so its semantics must
// match Valkey's: TTL expiry, NX writes and INCR windows.

func TestMemorySetGetExpire(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1000, 0)
	m := NewMemory()
	m.Now = func() time.Time { return now }

	if ok, err := m.SetJSON(ctx, "k", map[string]int{"a": 1}, 30*time.Second, false); !ok || err != nil {
		t.Fatalf("SetJSON = %v, %v", ok, err)
	}
	var got map[string]int
	if found, _ := m.GetJSON(ctx, "k", &got); !found || got["a"] != 1 {
		t.Fatalf("GetJSON = %v, %v", found, got)
	}
	now = now.Add(30 * time.Second)
	if found, _ := m.GetJSON(ctx, "k", &got); found {
		t.Fatal("value should have expired at its TTL")
	}
}

func TestMemoryOnlyIfAbsent(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if ok, _ := m.SetJSON(ctx, "k", 1, time.Minute, true); !ok {
		t.Fatal("first NX write must succeed")
	}
	if ok, _ := m.SetJSON(ctx, "k", 2, time.Minute, true); ok {
		t.Fatal("second NX write must not overwrite")
	}
	var v int
	_, _ = m.GetJSON(ctx, "k", &v)
	if v != 1 {
		t.Fatalf("value = %d, want 1", v)
	}
	_ = m.Delete(ctx, "k")
	if found, _ := m.GetJSON(ctx, "k", &v); found {
		t.Fatal("Delete did not remove key")
	}
}

func TestMemoryIncrWindow(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(0, 0)
	m := NewMemory()
	m.Now = func() time.Time { return now }
	for want := int64(1); want <= 3; want++ {
		if n, _ := m.Incr(ctx, "c", time.Minute); n != want {
			t.Fatalf("Incr = %d, want %d", n, want)
		}
	}
	now = now.Add(time.Minute)
	if n, _ := m.Incr(ctx, "c", time.Minute); n != 1 {
		t.Fatalf("counter should restart after window, got %d", n)
	}
}
