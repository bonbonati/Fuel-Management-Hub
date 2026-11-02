package events

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"
)

func TestNewEventEnvelope(t *testing.T) {
	ev, err := New("fueling", TypeTransactionCompleted, TransactionCompleted{TransactionID: "T-1", Amount: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(ev.ID) {
		t.Fatalf("id %q is not a v4 UUID", ev.ID)
	}
	if ev.Type != TypeTransactionCompleted || ev.Source != "fueling" || time.Since(ev.OccurredAt) > time.Minute {
		t.Fatalf("envelope = %+v", ev)
	}
	var p TransactionCompleted
	if err := ev.Decode(&p); err != nil || p.TransactionID != "T-1" || p.Amount != 5 {
		t.Fatalf("payload = %+v, %v", p, err)
	}
	if _, err := New("x", "y", func() {}); err == nil {
		t.Fatal("unmarshalable payload must error")
	}
}

// fakeOutbox records which ids the relay asked to mark as published.
type fakeOutbox struct {
	pending []Event
	marked  []string
}

func (f *fakeOutbox) ClaimBatch(_ context.Context, limit int, publish func([]Event) []string) error {
	batch := f.pending
	if len(batch) > limit {
		batch = batch[:limit]
	}
	f.marked = append(f.marked, publish(batch)...)
	return nil
}

type fakePublisher struct {
	failOn string
	sent   []string
}

func (f *fakePublisher) Publish(_ context.Context, ev Event) error {
	if ev.ID == f.failOn {
		return errors.New("broker down")
	}
	f.sent = append(f.sent, ev.ID)
	return nil
}

func TestRelayPublishesInOrderAndStopsAtFailure(t *testing.T) {
	store := &fakeOutbox{pending: []Event{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	pub := &fakePublisher{failOn: "b"}
	r := &Relay{store: store, pub: pub, batch: 10}

	n, err := r.Flush(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// "b" failed, so "c" must not be published ahead of it (ordering).
	if n != 1 || len(store.marked) != 1 || store.marked[0] != "a" {
		t.Fatalf("published %d, marked %v", n, store.marked)
	}
	if len(pub.sent) != 1 {
		t.Fatalf("sent %v", pub.sent)
	}
}

func TestRelayRespectsBatchSize(t *testing.T) {
	store := &fakeOutbox{pending: []Event{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	r := &Relay{store: store, pub: &fakePublisher{}, batch: 2}
	if n, _ := r.Flush(context.Background()); n != 2 {
		t.Fatalf("published %d, want batch size 2", n)
	}
}

func TestProcessAckDecisions(t *testing.T) {
	good, _ := json.Marshal(Event{ID: "1", Type: "t", Data: json.RawMessage(`{}`)})
	ok := func(context.Context, Event) error { return nil }
	fail := func(context.Context, Event) error { return errors.New("db down") }

	cases := []struct {
		name string
		body []byte
		h    Handler
		want ackAction
	}{
		{"handled", good, ok, actAck},
		{"transient failure requeues", good, fail, actRequeue},
		{"malformed json dead-letters", []byte("{"), ok, actDeadLetter},
		{"missing id dead-letters", []byte(`{"type":"t"}`), ok, actDeadLetter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := process(context.Background(), tc.body, tc.h); got != tc.want {
				t.Fatalf("action = %v, want %v", got, tc.want)
			}
		})
	}
}
