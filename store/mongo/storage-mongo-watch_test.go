package mongo

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
	testmongodb "xiaoshiai.cn/common/testkit/mongodb"
)

type Message struct {
	store.ObjectMeta `json:",inline"`
}

func (*Message) ResourceName() string {
	return "messages"
}

func TestMongoStorageWatchIntegration(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	database := RequireIntegrationDatabase(t, uri)
	storage := NewIntegrationStorage(t, database, &Message{})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	watcher, err := storage.Watch(ctx, &store.List[Message]{}, func(options *store.WatchOptions) {
		options.LabelRequirements = append(
			options.LabelRequirements,
			store.NewRequirement("example.com/team", store.Equals, "platform"),
		)
	})
	if err != nil {
		t.Fatalf("watch messages: %v", err)
	}
	defer watcher.Stop()

	message := &Message{ObjectMeta: store.ObjectMeta{
		ID:   "watch-message",
		Name: "watch integration test",
		Labels: map[string]string{
			"example.com/team": "platform",
		},
	}}
	if err := storage.Create(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}

	for {
		select {
		case event, ok := <-watcher.Events():
			if !ok {
				t.Fatal("watch closed before receiving the create event")
			}
			if event.Error != nil {
				t.Fatalf("watch event: %v", event.Error)
			}
			if event.Type == store.WatchEventCreate &&
				event.Object != nil &&
				event.Object.GetID() == message.GetID() {
				return
			}
		case <-ctx.Done():
			t.Fatalf("wait for create event: %v", ctx.Err())
		}
	}
}

func TestMongoStorageWatchInitialEventsAndSelectorTransitions(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	database := RequireIntegrationDatabase(t, uri)
	storage := NewIntegrationStorage(t, database, &Message{})

	existing := &Message{ObjectMeta: store.ObjectMeta{
		ID:     "existing",
		Labels: map[string]string{"team": "platform"},
	}}
	if err := storage.Create(t.Context(), existing); err != nil {
		t.Fatalf("create existing message: %v", err)
	}

	watcher, err := storage.Watch(
		t.Context(),
		&store.List[Message]{},
		store.WithSendInitialEvents(),
		func(options *store.WatchOptions) {
			options.LabelRequirements = append(
				options.LabelRequirements,
				store.NewRequirement("team", store.Equals, "platform"),
			)
		},
	)
	if err != nil {
		t.Fatalf("watch messages: %v", err)
	}
	defer watcher.Stop()

	assertMongoWatchEvent(t, watcher, store.WatchEventCreate, "existing")
	bookmark := nextMongoWatchEvent(t, watcher)
	if bookmark.Type != store.WatchEventBookmark || bookmark.ResourceVersion != 0 || bookmark.Object != nil {
		t.Fatalf("initial bookmark = %#v", bookmark)
	}

	existing.Labels["team"] = "other"
	if err := storage.Update(t.Context(), existing); err != nil {
		t.Fatalf("move existing message out of selector: %v", err)
	}
	deleted := assertMongoWatchEvent(t, watcher, store.WatchEventDelete, "existing")
	if deleted.Object.GetLabels()["team"] != "platform" {
		t.Fatalf("selector delete labels = %v, want previous platform label", deleted.Object.GetLabels())
	}

	existing.Labels["team"] = "platform"
	if err := storage.Update(t.Context(), existing); err != nil {
		t.Fatalf("move existing message into selector: %v", err)
	}
	assertMongoWatchEvent(t, watcher, store.WatchEventCreate, "existing")

	if err := storage.Delete(t.Context(), existing); err != nil {
		t.Fatalf("delete existing message: %v", err)
	}
	deleted = assertMongoWatchEvent(t, watcher, store.WatchEventDelete, "existing")
	if deleted.Object.GetUID() == "" || deleted.Object.GetResource() != "messages" {
		t.Fatalf("delete tombstone = %#v, want complete previous object", deleted.Object)
	}
}

func TestMongoStorageInitialBookmarkIncludesChangesDuringSnapshot(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	database := RequireIntegrationDatabase(t, uri)
	storage := NewIntegrationStorage(t, database, &Message{})

	const objectCount = 80
	objects := make([]*Message, 0, objectCount)
	for index := range objectCount {
		object := &Message{ObjectMeta: store.ObjectMeta{ID: fmt.Sprintf("snapshot-%03d", index)}}
		if err := storage.Create(t.Context(), object); err != nil {
			t.Fatalf("create initial message %d: %v", index, err)
		}
		objects = append(objects, object)
	}

	watcher, err := storage.Watch(t.Context(), &store.List[Message]{}, store.WithSendInitialEvents())
	if err != nil {
		t.Fatalf("watch messages: %v", err)
	}
	defer watcher.Stop()
	if err := storage.Delete(t.Context(), objects[0]); err != nil {
		t.Fatalf("delete during initial snapshot: %v", err)
	}

	initial := map[string]bool{}
	for {
		event := nextMongoWatchEvent(t, watcher)
		if event.Type == store.WatchEventBookmark {
			break
		}
		switch event.Type {
		case store.WatchEventCreate, store.WatchEventUpdate:
			initial[event.Object.GetID()] = true
		case store.WatchEventDelete:
			delete(initial, event.Object.GetID())
		}
	}
	if initial[objects[0].ID] {
		t.Fatalf("initial state contains %q deleted before Bookmark", objects[0].ID)
	}
}

func TestMongoStorageWatchReturnsResourceExpiredForUnavailableVersion(t *testing.T) {
	storage := &MongoStorage{}

	_, err := storage.Watch(t.Context(), &store.List[Message]{}, store.WithWatchResourceVersion(1))
	if !commonerrors.IsResourceExpired(err) {
		t.Fatalf("Watch() error = %v, want ResourceExpired", err)
	}
}

func TestMongoStorageWatchStopClosesEvents(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	database := RequireIntegrationDatabase(t, uri)
	storage := NewIntegrationStorage(t, database, &Message{})

	watcher, err := storage.Watch(t.Context(), &store.List[Message]{})
	if err != nil {
		t.Fatalf("watch messages: %v", err)
	}
	watcher.Stop()
	watcher.Stop()

	select {
	case _, ok := <-watcher.Events():
		if ok {
			t.Fatal("watch event channel remained open after Stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch event channel did not close after Stop")
	}
}

func nextMongoWatchEvent(t *testing.T, watcher store.Watcher) store.WatchEvent {
	t.Helper()
	select {
	case event, ok := <-watcher.Events():
		if !ok {
			t.Fatal("watch closed before the expected event")
		}
		if event.Error != nil {
			t.Fatalf("watch event error: %v", event.Error)
		}
		return event
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for watch event")
		return store.WatchEvent{}
	}
}

func assertMongoWatchEvent(t *testing.T, watcher store.Watcher, eventType store.WatchEventType, id string) store.WatchEvent {
	t.Helper()
	event := nextMongoWatchEvent(t, watcher)
	if event.Type != eventType || event.Object == nil || event.Object.GetID() != id {
		t.Fatalf("watch event = %#v, want type %q and id %q", event, eventType, id)
	}
	return event
}

func TestToWatchFilterMatchesCurrentOrPreviousImmutableFields(t *testing.T) {
	want := bson.D{
		{Key: "operationType", Value: bson.M{"$in": bson.A{"insert", "update", "replace", "delete"}}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "fullDocument.tenant", Value: "acme"}},
			bson.D{{Key: "fullDocumentBeforeChange.tenant", Value: "acme"}},
		}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "fullDocument.id", Value: "message"}},
			bson.D{{Key: "fullDocumentBeforeChange.id", Value: "message"}},
		}},
	}

	got := ToWatchFilter(bson.D{{Key: "tenant", Value: "acme"}, {Key: "id", Value: "message"}})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("watch filter = %#v, want %#v", got, want)
	}
}
