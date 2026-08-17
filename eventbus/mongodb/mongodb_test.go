package mongodb_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"xiaoshiai.cn/common/eventbus"
	eventmongodb "xiaoshiai.cn/common/eventbus/mongodb"
	testmongodb "xiaoshiai.cn/common/testkit/mongodb"
)

func TestPublishStoresOneEventWithServerAcceptanceTime(t *testing.T) {
	test := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	test.Run("publish", func(mt *mtest.T) {
		acceptedAt := primitive.Timestamp{
			T: 10,
			I: 1,
		}
		eventTime := time.Now().UTC().Truncate(time.Millisecond)
		mt.AddMockResponses(
			mtest.CreateSuccessResponse(),
			mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{
				{Key: "_id", Value: "event-1"},
				{Key: "type", Value: "order.created.v1"},
				{Key: "source", Value: "orders"},
				{Key: "subject", Value: "order-1"},
				{Key: "time", Value: eventTime},
				{Key: "acceptedAt", Value: acceptedAt},
				{Key: "data", Value: []byte("payload")},
				{Key: "metadata", Value: bson.D{}},
				{Key: "idempotencyKey", Value: "operation-1"},
				{Key: "consumptions", Value: bson.D{}},
			}}),
		)
		bus, err := eventmongodb.New(
			mt.Context(),
			mt.Coll,
			mt.DB.Collection(mt.Coll.Name()+"_consumers"),
			eventmongodb.Options{},
		)
		if err != nil {
			mt.Fatalf("New() error = %v", err)
		}
		id, err := bus.Publish(mt.Context(), eventbus.Event{
			ID:      "event-1",
			Type:    "order.created.v1",
			Source:  "orders",
			Subject: "order-1",
			Time:    eventTime,
			Data:    []byte("payload"),
		}, eventbus.PublishOptions{IdempotencyKey: "operation-1"})
		if err != nil {
			mt.Fatalf("Publish() error = %v", err)
		}
		if id != "event-1" {
			mt.Fatalf("Publish() ID = %q, want event-1", id)
		}

		command := findCommand(mt, mt.Coll.Name())
		encoded, err := bson.MarshalExtJSON(command, false, false)
		if err != nil {
			mt.Fatalf("MarshalExtJSON() error = %v", err)
		}
		query := string(encoded)
		for _, expected := range []string{"$$CLUSTER_TIME", "consumptions", "operation-1"} {
			if !strings.Contains(query, expected) {
				mt.Fatalf("publish command %s does not contain %q", query, expected)
			}
		}
	})
}

func TestConsumptionPathDependsOnlyOnConsumerID(t *testing.T) {
	test := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	test.Run("subscribe", func(mt *mtest.T) {
		consumerID := "billing"
		consumerKey := fmt.Sprintf("%x", sha256.Sum256([]byte(consumerID)))
		activatedAt := primitive.Timestamp{
			T: 10,
			I: 1,
		}
		acceptedAt := primitive.Timestamp{
			T: 10,
			I: 2,
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		mt.AddMockResponses(
			mtest.CreateSuccessResponse(),
			mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{
				{Key: "_id", Value: consumerKey},
				{Key: "consumerId", Value: consumerID},
				{Key: "activatedAt", Value: activatedAt},
				{Key: "ephemeral", Value: false},
			}}),
			mtest.CreateSuccessResponse(
				bson.E{Key: "n", Value: 0},
				bson.E{Key: "nModified", Value: 0},
			),
			mtest.CreateSuccessResponse(
				bson.E{Key: "n", Value: 0},
				bson.E{Key: "nModified", Value: 0},
			),
			mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{
				{Key: "_id", Value: "event-1"},
				{Key: "type", Value: "order.created.v1"},
				{Key: "time", Value: now},
				{Key: "acceptedAt", Value: acceptedAt},
				{Key: "data", Value: []byte("payload")},
				{Key: "metadata", Value: bson.D{}},
				{Key: "consumptions", Value: bson.D{
					{Key: consumerKey, Value: bson.D{
						{Key: "state", Value: "Running"},
						{Key: "attempt", Value: 1},
						{Key: "notBefore", Value: now},
					}},
				}},
			}}),
		)
		consumers := mt.DB.Collection(mt.Coll.Name() + "_consumers")
		bus, err := eventmongodb.New(mt.Context(), mt.Coll, consumers, eventmongodb.Options{
			PollInterval:  time.Hour,
			LeaseDuration: time.Hour,
		})
		if err != nil {
			mt.Fatalf("New() error = %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		handled := make(chan eventbus.Event, 1)
		done := make(chan error, 1)
		go func() {
			done <- bus.Subscribe(ctx, "order.**", eventbus.HandlerFunc(func(
				_ context.Context,
				event eventbus.Event,
			) error {
				handled <- event
				cancel()
				return nil
			}), eventbus.SubscribeOptions{ConsumerID: consumerID})
		}()
		select {
		case event := <-handled:
			if event.ID != "event-1" {
				mt.Fatalf("handled event ID = %q, want event-1", event.ID)
			}
		case <-time.After(time.Second):
			mt.Fatal("handler was not called")
		}
		select {
		case err := <-done:
			if err != nil {
				mt.Fatalf("Subscribe() error = %v", err)
			}
		case <-time.After(time.Second):
			mt.Fatal("Subscribe() did not stop")
		}

		command := findCommand(mt, mt.Coll.Name())
		encoded, err := bson.MarshalExtJSON(command, false, false)
		if err != nil {
			mt.Fatalf("MarshalExtJSON() error = %v", err)
		}
		query := string(encoded)
		if !strings.Contains(query, "consumptions."+consumerKey) {
			mt.Fatalf("claim command %s does not use ConsumerID key", query)
		}
		wrongKey := fmt.Sprintf("%x", sha256.Sum256([]byte("order.**\x00"+consumerID)))
		if strings.Contains(query, wrongKey) {
			mt.Fatalf("claim command %s includes pattern-dependent key", query)
		}
	})
}

func findCommand(mt *mtest.T, collection string) bson.Raw {
	mt.Helper()
	var found bson.Raw
	for _, event := range mt.GetAllStartedEvents() {
		if event.CommandName != "findAndModify" {
			continue
		}
		name, ok := event.Command.Lookup("findAndModify").StringValueOK()
		if ok && name == collection {
			found = event.Command
		}
	}
	if found == nil {
		mt.Fatalf("findAndModify command for %q was not sent", collection)
	}
	return found
}

func TestIntegrationMongoDB(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	t.Run("publish is idempotent", func(t *testing.T) {
		testIntegrationPublishIsIdempotent(t, uri)
	})
	t.Run("consumer ID owns progress across patterns", func(t *testing.T) {
		testIntegrationConsumerIDOwnsProgressAcrossPatterns(t, uri)
	})
	t.Run("durable consumer resumes pending progress", func(t *testing.T) {
		testIntegrationDurableConsumerResumesPendingProgress(t, uri)
	})
	t.Run("retry and lease recovery", func(t *testing.T) {
		testIntegrationRetryAndLeaseRecovery(t, uri)
	})
}

func testIntegrationPublishIsIdempotent(t *testing.T, uri string) {
	fixture := newIntegrationBus(t, uri, eventmongodb.Options{})
	event := eventbus.Event{
		Type: "order.created.v1",
		Data: []byte("order-1"),
	}
	options := eventbus.PublishOptions{
		IdempotencyKey: "create/order-1",
	}

	firstID, err := fixture.Bus.Publish(t.Context(), event, options)
	if err != nil {
		t.Fatalf("first Publish() error = %v", err)
	}
	secondID, err := fixture.Bus.Publish(t.Context(), event, options)
	if err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}
	if secondID != firstID {
		t.Fatalf("second Publish() ID = %q, want %q", secondID, firstID)
	}

	conflict := event
	conflict.Data = []byte("another-order")
	if _, err := fixture.Bus.Publish(t.Context(), conflict, options); !errors.Is(err, eventbus.ErrConflict) {
		t.Fatalf("conflicting Publish() error = %v, want ErrConflict", err)
	}
}

func testIntegrationConsumerIDOwnsProgressAcrossPatterns(t *testing.T, uri string) {
	fixture := newIntegrationBus(t, uri, fastIntegrationOptions())
	consumerID := "shared"
	firstEvents := make(chan eventbus.Event, 4)
	secondEvents := make(chan eventbus.Event, 4)
	first := startIntegrationSubscriber(
		t,
		fixture.Bus,
		"order.*",
		consumerID,
		firstEvents,
	)
	defer first.Stop(t)
	second := startIntegrationSubscriber(
		t,
		fixture.Bus,
		"*.created",
		consumerID,
		secondEvents,
	)
	defer second.Stop(t)
	requireIntegrationConsumer(t, fixture.Consumers, consumerID)

	firstOnlyID := publishIntegrationEvent(t, fixture.Bus, "order.updated")
	requireIntegrationEvent(t, firstEvents, firstOnlyID)
	secondOnlyID := publishIntegrationEvent(t, fixture.Bus, "user.created")
	requireIntegrationEvent(t, secondEvents, secondOnlyID)

	overlapID := publishIntegrationEvent(t, fixture.Bus, "order.created")
	select {
	case event := <-firstEvents:
		if event.ID != overlapID {
			t.Fatalf("first subscriber received event %q, want %q", event.ID, overlapID)
		}
	case event := <-secondEvents:
		if event.ID != overlapID {
			t.Fatalf("second subscriber received event %q, want %q", event.ID, overlapID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("overlapping subscribers did not receive the event")
	}
	requireIntegrationConsumption(t, fixture.Events, overlapID, consumerID, "Acked", 1)
	select {
	case event := <-firstEvents:
		t.Fatalf("overlapping event was delivered again to first subscriber as %q", event.ID)
	case event := <-secondEvents:
		t.Fatalf("overlapping event was delivered again to second subscriber as %q", event.ID)
	case <-time.After(100 * time.Millisecond):
	}
}

func testIntegrationDurableConsumerResumesPendingProgress(t *testing.T, uri string) {
	fixture := newIntegrationBus(t, uri, fastIntegrationOptions())
	consumerID := "billing"
	firstEvents := make(chan eventbus.Event, 1)
	first := startIntegrationSubscriber(t, fixture.Bus, "invoice.**", consumerID, firstEvents)
	requireIntegrationConsumer(t, fixture.Consumers, consumerID)
	first.Stop(t)

	eventID := publishIntegrationEvent(t, fixture.Bus, "invoice.created")
	resumedEvents := make(chan eventbus.Event, 1)
	resumed := startIntegrationSubscriber(t, fixture.Bus, "invoice.*", consumerID, resumedEvents)
	defer resumed.Stop(t)
	requireIntegrationEvent(t, resumedEvents, eventID)
	requireIntegrationConsumption(t, fixture.Events, eventID, consumerID, "Acked", 1)

	newEvents := make(chan eventbus.Event, 1)
	newConsumer := startIntegrationSubscriber(t, fixture.Bus, "invoice.**", "new-consumer", newEvents)
	defer newConsumer.Stop(t)
	requireIntegrationConsumer(t, fixture.Consumers, "new-consumer")
	select {
	case event := <-newEvents:
		t.Fatalf("new consumer replayed event %q accepted before activation", event.ID)
	case <-time.After(100 * time.Millisecond):
	}
}

func testIntegrationRetryAndLeaseRecovery(t *testing.T, uri string) {
	t.Run("handler retry", func(t *testing.T) {
		fixture := newIntegrationBus(t, uri, fastIntegrationOptions())
		consumerID := "retrying"
		var calls atomic.Int32
		delivered := make(chan eventbus.Event, 1)
		subscriber := startIntegrationSubscriberWithHandler(
			t,
			fixture.Bus,
			"payment.*",
			consumerID,
			eventbus.HandlerFunc(func(_ context.Context, event eventbus.Event) error {
				if calls.Add(1) == 1 {
					return eventbus.RetryAfter(errors.New("temporary"), 10*time.Millisecond)
				}
				delivered <- event
				return nil
			}),
		)
		defer subscriber.Stop(t)
		requireIntegrationConsumer(t, fixture.Consumers, consumerID)

		eventID := publishIntegrationEvent(t, fixture.Bus, "payment.created")
		requireIntegrationEvent(t, delivered, eventID)
		requireIntegrationConsumption(t, fixture.Events, eventID, consumerID, "Acked", 2)
	})

	t.Run("expired lease", func(t *testing.T) {
		options := fastIntegrationOptions()
		options.LeaseDuration = 100 * time.Millisecond
		fixture := newIntegrationBus(t, uri, options)
		consumerID := "recovering"
		started := make(chan eventbus.Event, 1)
		first := startIntegrationSubscriberWithHandler(
			t,
			fixture.Bus,
			"shipment.*",
			consumerID,
			eventbus.HandlerFunc(func(ctx context.Context, event eventbus.Event) error {
				started <- event
				<-ctx.Done()
				return ctx.Err()
			}),
		)
		requireIntegrationConsumer(t, fixture.Consumers, consumerID)

		eventID := publishIntegrationEvent(t, fixture.Bus, "shipment.created")
		requireIntegrationEvent(t, started, eventID)
		first.Stop(t)

		recovered := make(chan eventbus.Event, 1)
		second := startIntegrationSubscriber(t, fixture.Bus, "shipment.*", consumerID, recovered)
		defer second.Stop(t)
		requireIntegrationEvent(t, recovered, eventID)
		requireIntegrationConsumption(t, fixture.Events, eventID, consumerID, "Acked", 2)
	})
}

type integrationFixture struct {
	Bus       *eventmongodb.Bus
	Events    *mongo.Collection
	Consumers *mongo.Collection
}

func newIntegrationBus(
	t *testing.T,
	uri string,
	options eventmongodb.Options,
) integrationFixture {
	t.Helper()
	database := testmongodb.RequireDatabase(t, uri)
	events := database.Collection("events")
	consumers := database.Collection("consumers")
	bus, err := eventmongodb.New(t.Context(), events, consumers, options)
	if err != nil {
		t.Fatalf("create MongoDB event bus: %v", err)
	}
	return integrationFixture{
		Bus:       bus,
		Events:    events,
		Consumers: consumers,
	}
}

func fastIntegrationOptions() eventmongodb.Options {
	return eventmongodb.Options{
		PollInterval:  10 * time.Millisecond,
		LeaseDuration: time.Second,
	}
}

type integrationSubscriber struct {
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
}

func startIntegrationSubscriber(
	t *testing.T,
	bus *eventmongodb.Bus,
	pattern string,
	consumerID string,
	events chan<- eventbus.Event,
) *integrationSubscriber {
	t.Helper()
	return startIntegrationSubscriberWithHandler(
		t,
		bus,
		pattern,
		consumerID,
		eventbus.HandlerFunc(func(_ context.Context, event eventbus.Event) error {
			events <- event
			return nil
		}),
	)
}

func startIntegrationSubscriberWithHandler(
	t *testing.T,
	bus *eventmongodb.Bus,
	pattern string,
	consumerID string,
	handler eventbus.Handler,
) *integrationSubscriber {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	running := &integrationSubscriber{
		cancel: cancel,
		done:   make(chan error, 1),
	}
	go func() {
		running.done <- bus.Subscribe(ctx, pattern, handler, eventbus.SubscribeOptions{
			ConsumerID: consumerID,
		})
	}()
	return running
}

func (s *integrationSubscriber) Stop(t *testing.T) {
	t.Helper()
	s.once.Do(func() {
		s.cancel()
		select {
		case err := <-s.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Subscribe() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Subscribe() did not stop")
		}
	})
}

func publishIntegrationEvent(t *testing.T, bus *eventmongodb.Bus, eventType string) string {
	t.Helper()
	id, err := bus.Publish(t.Context(), eventbus.Event{Type: eventType}, eventbus.PublishOptions{})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	return id
}

func requireIntegrationEvent(t *testing.T, events <-chan eventbus.Event, id string) {
	t.Helper()
	select {
	case event := <-events:
		if event.ID != id {
			t.Fatalf("received event ID = %q, want %q", event.ID, id)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for event %q", id)
	}
}

func requireIntegrationConsumer(t *testing.T, consumers *mongo.Collection, consumerID string) {
	t.Helper()
	requireIntegrationCondition(t, func() (bool, error) {
		count, err := consumers.CountDocuments(t.Context(), bson.D{
			{Key: "consumerId", Value: consumerID},
		})
		return count == 1, err
	})
}

func requireIntegrationConsumption(
	t *testing.T,
	events *mongo.Collection,
	eventID string,
	consumerID string,
	state string,
	attempt int,
) {
	t.Helper()
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(consumerID)))
	path := "consumptions." + key
	requireIntegrationCondition(t, func() (bool, error) {
		count, err := events.CountDocuments(t.Context(), bson.D{
			{Key: "_id", Value: eventID},
			{Key: path + ".state", Value: state},
			{Key: path + ".attempt", Value: attempt},
		})
		return count == 1, err
	})
}

func requireIntegrationCondition(t *testing.T, condition func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		matched, err := condition()
		if err != nil {
			t.Fatalf("inspect integration state: %v", err)
		}
		if matched {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for MongoDB integration state")
}
