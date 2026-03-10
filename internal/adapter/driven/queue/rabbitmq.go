package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/hack-fiap233/videos/internal/application"
)

// VideoJobPayload corpo da mensagem na fila video.process
type VideoJobPayload struct {
	VideoID    int    `json:"video_id"`
	UserID     int    `json:"user_id"`
	UserEmail  string `json:"user_email"`
	StorageKey string `json:"storage_key"`
}

// VideoQueue usando RabbitMQ.
type RabbitMQQueue struct {
	conn    *amqp.Connection
	ch      *amqp.Channel
	queue   string
	queueDLQ string
	mu      sync.Mutex
}

var _ application.VideoQueue = (*RabbitMQQueue)(nil)

// NewRabbitMQQueue conecta ao RabbitMQ e declara as filas (process + DLQ).
// amqpURL ex.: "amqp://user:pass@host:5672/"
func NewRabbitMQQueue(amqpURL, queueName, dlqName string) (*RabbitMQQueue, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("amqp channel: %w", err)
	}
	_, err = ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("queue declare %s: %w", queueName, err)
	}
	_, err = ch.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("queue declare DLQ %s: %w", dlqName, err)
	}
	return &RabbitMQQueue{conn: conn, ch: ch, queue: queueName, queueDLQ: dlqName}, nil
}

func (q *RabbitMQQueue) PublishVideoJob(ctx context.Context, videoID, userID int, userEmail, storageKey string) error {
	payload := VideoJobPayload{VideoID: videoID, UserID: userID, UserEmail: userEmail, StorageKey: storageKey}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.ch.PublishWithContext(ctx, "", q.queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
}

// Close fecha conexão e canal
func (q *RabbitMQQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	var err error
	if q.ch != nil {
		err = q.ch.Close()
		q.ch = nil
	}
	if q.conn != nil {
		if e := q.conn.Close(); e != nil && err == nil {
			err = e
		}
		q.conn = nil
	}
	return err
}

// Consume retorna um canal de mensagens para o worker consumir.
// O caller deve fazer ack/nack. Útil para o worker.
func (q *RabbitMQQueue) Consume(prefetch int) (<-chan amqp.Delivery, error) {
	if err := q.ch.Qos(prefetch, 0, false); err != nil {
		return nil, err
	}
	return q.ch.Consume(q.queue, "", false, false, false, false, nil)
}

// QueueName retorna o nome da fila principal (para logs)
func (q *RabbitMQQueue) QueueName() string { return q.queue }

// DLQName retorna o nome da DLQ.
func (q *RabbitMQQueue) DLQName() string { return q.queueDLQ }
