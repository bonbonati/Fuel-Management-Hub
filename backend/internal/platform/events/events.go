// Package events implements asynchronous domain events between services.
//
// Delivery guarantees: at-least-once. A producer never publishes to the
// broker inside a request. It writes the event into its own "outbox" table in
// the same database transaction as the state change (transactional outbox
// pattern; see the outbox table in internal/fueling/migrations), and a background Relay publishes pending rows to RabbitMQ. This
// way a committed state change always produces its event, and a rolled back
// one never does. Consumers must therefore be idempotent (dedupe by Event.ID).
//
// Topology: one durable topic exchange (default "petro.events"); routing key =
// Event.Type, e.g. "fueling.transaction.completed". Each consumer owns a
// durable quorum queue with a delivery limit and a dead-letter queue, so a
// poison message is parked after a few attempts instead of looping forever.
package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Event is the envelope published on the bus.
type Event struct {
	ID         string          `json:"id"`         // unique id, used by consumers for deduplication
	Type       string          `json:"type"`       // routing key: <service>.<entity>.<verb-past-tense>
	Source     string          `json:"source"`     // producing service
	OccurredAt time.Time       `json:"occurredAt"` // when the state change happened
	Data       json.RawMessage `json:"data"`       // event-specific payload
}

// New builds an event with a random id and the current time.
func New(source, typ string, data any) (Event, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return Event{}, fmt.Errorf("events: marshal %s: %w", typ, err)
	}
	return Event{ID: newID(), Type: typ, Source: source, OccurredAt: time.Now().UTC(), Data: b}, nil
}

// Decode unmarshals the payload into dst.
func (e Event) Decode(dst any) error { return json.Unmarshal(e.Data, dst) }

// newID returns a random UUIDv4 string (Postgres stores it as uuid).
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Event types published on the bus. Payload structs live next to them so
// producers and consumers share one definition.
const (
	// TypeTransactionCompleted is emitted by fueling when a fill-up finishes.
	TypeTransactionCompleted = "fueling.transaction.completed"
)

// TransactionCompleted is the payload of TypeTransactionCompleted.
type TransactionCompleted struct {
	TransactionID string    `json:"transactionId"`
	StationID     string    `json:"stationId"`
	OrgID         string    `json:"orgId,omitempty"` // empty for individual (non-fleet) customers
	Amount        int64     `json:"amount"`          // SDG, VAT inclusive
	CompletedAt   time.Time `json:"completedAt"`
}
