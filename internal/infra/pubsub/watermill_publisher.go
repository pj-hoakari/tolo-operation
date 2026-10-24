package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-googlecloud/v2/pkg/googlecloud"
	"github.com/ThreeDotsLabs/watermill-nats/v2/pkg/jetstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/nats-io/nats.go"
	natsjetstream "github.com/nats-io/nats.go/jetstream"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

const EventIDMetadataKey = "event_id"

var topics = []domain.Topic{domain.TopicGuestMessages, domain.TopicRealtimeDelivery}

type OutboxPublisher struct {
	publisher message.Publisher
}

func NewOutboxPublisher(publisher message.Publisher) *OutboxPublisher {
	return &OutboxPublisher{publisher: publisher}
}

func (p *OutboxPublisher) Publish(ctx context.Context, outbox domain.OutboxMessage) error {
	msg := message.NewMessage(watermill.NewUUID(), outbox.Payload)
	msg.Metadata.Set(EventIDMetadataKey, outbox.OrderingKey)
	msg.SetContext(ctx)

	if err := p.publisher.Publish(string(outbox.Topic), msg); err != nil {
		return fmt.Errorf("publish to %s: %w", outbox.Topic, err)
	}

	return nil
}

func NewNATSPublisher(ctx context.Context, url string, maxAge time.Duration) (message.Publisher, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	if err := provisionStreams(ctx, conn, maxAge); err != nil {
		conn.Close()

		return nil, err
	}

	publisher, err := jetstream.NewPublisher(jetstream.PublisherConfig{
		URL:             "",
		Conn:            conn,
		Logger:          watermill.NewSlogLogger(slog.Default()),
		ConfigureStream: nil,
		TrackMessageID:  false,
	})
	if err != nil {
		conn.Close()

		return nil, fmt.Errorf("create NATS JetStream publisher: %w", err)
	}

	return publisher, nil
}

func provisionStreams(ctx context.Context, conn *nats.Conn, maxAge time.Duration) error {
	js, err := natsjetstream.New(conn)
	if err != nil {
		return fmt.Errorf("open JetStream: %w", err)
	}

	for _, topic := range topics {
		if _, err := js.CreateOrUpdateStream(ctx, natsjetstream.StreamConfig{
			Name:     string(topic),
			Subjects: []string{string(topic)},
			MaxAge:   maxAge,
		}); err != nil {
			return fmt.Errorf("provision the %s stream: %w", topic, err)
		}
	}

	return nil
}

func NewGooglePublisher(projectID string) (message.Publisher, error) {
	publisher, err := googlecloud.NewPublisher(googlecloud.PublisherConfig{
		ProjectID:                 projectID,
		DoNotCheckTopicExistence:  false,
		DoNotCreateTopicIfMissing: false,
		EnableMessageOrdering:     true,
		EnableMessageOrderingAutoResumePublishOnError: true,
		ConnectTimeout:  0,
		PublishTimeout:  0,
		PublishSettings: nil,
		ClientOptions:   nil,
		ClientConfig:    nil,
		Marshaler: googlecloud.NewOrderingMarshaler(func(_ string, msg *message.Message) (string, error) {
			return msg.Metadata.Get(EventIDMetadataKey), nil
		}),
	}, watermill.NewSlogLogger(slog.Default()))
	if err != nil {
		return nil, fmt.Errorf("create Cloud Pub/Sub publisher: %w", err)
	}

	return publisher, nil
}
