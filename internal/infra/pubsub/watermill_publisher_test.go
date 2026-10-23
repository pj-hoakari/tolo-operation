package pubsub_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	gcpubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/testcontainers/testcontainers-go"
	tcpubsub "github.com/testcontainers/testcontainers-go/modules/gcloud/pubsub"
	"google.golang.org/protobuf/proto"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/infra/pubsub"
)

const emulatorImage = "gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators"

func TestGoogleCloudPublisherKeepsOrderPerEventID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := tcpubsub.Run(ctx, emulatorImage)
	testcontainers.CleanupContainer(t, container)

	if err != nil {
		t.Fatalf("start Pub/Sub emulator: %v", err)
	}

	t.Setenv("PUBSUB_EMULATOR_HOST", container.URI())

	project := container.ProjectID()
	topic := string(domain.TopicGuestMessages)
	subscription := subscribe(ctx, t, project, topic)

	google, err := pubsub.NewGooglePublisher(project)
	if err != nil {
		t.Fatalf("NewGooglePublisher: %v", err)
	}

	t.Cleanup(func() {
		if err := google.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	publisher := pubsub.NewOutboxPublisher(google)

	eventIDs := []string{"event-a", "event-b"}
	sent := map[string][]*operationv1.GuestMessagesUpdate{}

	for n := range 3 {
		for _, eventID := range eventIDs {
			update := &operationv1.GuestMessagesUpdate{EventId: eventID, RevokedMessageIds: []string{fmt.Sprintf("message-%d", n)}}

			message, err := domain.NewOutboxMessage(domain.TopicGuestMessages, eventID, update)
			if err != nil {
				t.Fatalf("NewOutboxMessage: %v", err)
			}

			if err := publisher.Publish(ctx, message); err != nil {
				t.Fatalf("Publish %s #%d: %v", eventID, n, err)
			}

			sent[eventID] = append(sent[eventID], update)
		}
	}

	received := receive(ctx, t, subscription, 6)

	for _, eventID := range eventIDs {
		got := received[eventID]
		if !slices.EqualFunc(got, sent[eventID], func(a, b *operationv1.GuestMessagesUpdate) bool { return proto.Equal(a, b) }) {
			t.Errorf("ordering key %s received %v, want %v in publish order", eventID, got, sent[eventID])
		}
	}
}

func subscribe(ctx context.Context, t *testing.T, project, topic string) *gcpubsub.Subscriber {
	t.Helper()

	client, err := gcpubsub.NewClient(ctx, project)
	if err != nil {
		t.Fatalf("connect to the emulator: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	topicName := fmt.Sprintf("projects/%s/topics/%s", project, topic)
	if _, err := client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName}); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	subscriptionName := fmt.Sprintf("projects/%s/subscriptions/%s-test", project, topic)
	if _, err := client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:                  subscriptionName,
		Topic:                 topicName,
		EnableMessageOrdering: true,
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	return client.Subscriber(subscriptionName)
}

func receive(ctx context.Context, t *testing.T, subscriber *gcpubsub.Subscriber, want int) map[string][]*operationv1.GuestMessagesUpdate {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	var (
		mu       sync.Mutex
		count    int
		received = map[string][]*operationv1.GuestMessagesUpdate{}
	)

	err := subscriber.Receive(ctx, func(_ context.Context, msg *gcpubsub.Message) {
		msg.Ack()

		var update operationv1.GuestMessagesUpdate
		if err := proto.Unmarshal(msg.Data, &update); err != nil {
			t.Errorf("decode %x: %v", msg.Data, err)
		}

		mu.Lock()
		defer mu.Unlock()

		received[msg.OrderingKey] = append(received[msg.OrderingKey], &update)

		if count++; count == want {
			cancel()
		}
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if count != want {
		t.Fatalf("received %d messages, want %d", count, want)
	}

	return received
}
