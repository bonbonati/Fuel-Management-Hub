package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DefaultExchange is the topic exchange every service publishes to.
const DefaultExchange = "petro.events"

// AMQPPublisher publishes events to RabbitMQ with publisher confirms, so a
// publish only succeeds once the broker has taken responsibility for it. The
// connection is opened lazily and re-opened after failures.
type AMQPPublisher struct {
	url, exchange string

	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewAMQPPublisher returns a publisher for the given broker URL and exchange.
func NewAMQPPublisher(url, exchange string) *AMQPPublisher {
	return &AMQPPublisher{url: url, exchange: exchange}
}

func (p *AMQPPublisher) channel() (*amqp.Channel, error) {
	if p.ch != nil && !p.ch.IsClosed() {
		return p.ch, nil
	}
	p.closeLocked()
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err == nil {
		err = ch.ExchangeDeclare(p.exchange, amqp.ExchangeTopic, true, false, false, false, nil)
	}
	if err == nil {
		err = ch.Confirm(false)
	}
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	p.conn, p.ch = conn, ch
	return ch, nil
}

// Publish implements Publisher.
func (p *AMQPPublisher) Publish(ctx context.Context, ev Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, err := p.channel()
	if err != nil {
		return fmt.Errorf("events: connect: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conf, err := ch.PublishWithDeferredConfirmWithContext(ctx, p.exchange, ev.Type, true, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    ev.ID,
		Type:         ev.Type,
		AppId:        ev.Source,
		Timestamp:    ev.OccurredAt,
		Body:         body,
	})
	if err != nil {
		p.closeLocked()
		return err
	}
	ok, err := conf.WaitContext(ctx)
	if err != nil {
		p.closeLocked()
		return err
	}
	if !ok {
		return errors.New("events: broker nacked publish")
	}
	return nil
}

// Close releases the connection.
func (p *AMQPPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *AMQPPublisher) closeLocked() {
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.conn, p.ch = nil, nil
}

// Handler processes one event. Returning an error requeues the message;
// after maxDeliveries attempts RabbitMQ dead-letters it.
type Handler func(ctx context.Context, ev Event) error

// maxDeliveries bounds redeliveries before a message is dead-lettered.
const maxDeliveries = 5

// ackAction is what to do with a delivery after handling it.
type ackAction int

const (
	actAck        ackAction = iota // processed
	actRequeue                     // transient failure, try again
	actDeadLetter                  // malformed, never retry
)

// process decodes and handles one message body. It is separated from the AMQP
// plumbing so the retry/dead-letter decisions are unit-testable.
func process(ctx context.Context, body []byte, h Handler) ackAction {
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" || ev.Type == "" {
		slog.Error("dropping malformed event", "err", err)
		return actDeadLetter
	}
	if err := h(ctx, ev); err != nil {
		slog.Warn("event handler failed; requeueing", "type", ev.Type, "id", ev.ID, "err", err)
		return actRequeue
	}
	return actAck
}

// Consumer subscribes a handler to routing keys on a durable queue.
type Consumer struct {
	URL, Exchange, Queue string
	Keys                 []string
	Handler              Handler
}

// Run consumes until ctx is cancelled, reconnecting with backoff when the
// broker connection drops (e.g. during a RabbitMQ restart).
func (c Consumer) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("event consumer disconnected; reconnecting", "queue", c.Queue, "err", err, "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (c Consumer) runOnce(ctx context.Context) error {
	conn, err := amqp.Dial(c.URL)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	if err := c.declare(ch); err != nil {
		return err
	}
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.ConsumeWithContext(ctx, c.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	slog.Info("event consumer started", "queue", c.Queue, "keys", c.Keys)
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-closed:
			return e
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			switch process(ctx, d.Body, c.Handler) {
			case actAck:
				_ = d.Ack(false)
			case actRequeue:
				_ = d.Nack(false, true)
			case actDeadLetter:
				_ = d.Nack(false, false)
			}
		}
	}
}

// declare creates the exchange, the dead-letter exchange/queue, and the
// consumer's quorum queue with its bindings. All declarations are idempotent.
func (c Consumer) declare(ch *amqp.Channel) error {
	dlx := c.Exchange + ".dlx"
	if err := ch.ExchangeDeclare(c.Exchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(dlx, amqp.ExchangeFanout, true, false, false, false, nil); err != nil {
		return err
	}
	dlq := c.Queue + ".dlq"
	if _, err := ch.QueueDeclare(dlq, true, false, false, false, amqp.Table{"x-queue-type": "quorum"}); err != nil {
		return err
	}
	if err := ch.QueueBind(dlq, "#", dlx, false, nil); err != nil {
		return err
	}
	args := amqp.Table{
		"x-queue-type":           "quorum",
		"x-delivery-limit":       maxDeliveries,
		"x-dead-letter-exchange": dlx,
	}
	if _, err := ch.QueueDeclare(c.Queue, true, false, false, false, args); err != nil {
		return err
	}
	for _, k := range c.Keys {
		if err := ch.QueueBind(c.Queue, k, c.Exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}
