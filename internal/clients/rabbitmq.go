package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// QueueName is the name of the notifications queue.
	QueueName = "alkemio-notifications"
)

// RabbitMQClient wraps the RabbitMQ connection for publishing messages.
type RabbitMQClient struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	mu      sync.Mutex
}

// NewRabbitMQClient creates a new RabbitMQ client.
func NewRabbitMQClient(url string) (*RabbitMQClient, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare the queue (idempotent)
	_, err = ch.QueueDeclare(
		QueueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	return &RabbitMQClient{
		conn:    conn,
		channel: ch,
	}, nil
}

// Ping checks connectivity to RabbitMQ.
func (c *RabbitMQClient) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.conn.IsClosed() {
		return fmt.Errorf("connection closed")
	}
	if c.channel == nil || c.channel.IsClosed() {
		return fmt.Errorf("channel closed")
	}
	return nil
}

// Close closes the RabbitMQ connection.
func (c *RabbitMQClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Publish sends a message to the notifications queue.
func (c *RabbitMQClient) Publish(ctx context.Context, event any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	err = c.channel.PublishWithContext(
		ctx,
		"",        // exchange
		QueueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}
