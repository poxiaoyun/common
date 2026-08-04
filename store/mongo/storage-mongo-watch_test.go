package mongo

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"xiaoshiai.cn/common/store"
)

type Message struct {
	store.ObjectMeta `json:",inline"`
}

func (*Message) ResourceName() string {
	return "messages"
}

func TestMongoStorageWatchIntegration(t *testing.T) {
	options := mongoIntegrationOptions(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := store.NewSchema()
	if err := schema.Register(&Message{}, store.ResourceSchema{}); err != nil {
		t.Fatalf("register schema: %v", err)
	}
	storage, err := NewMongoStorage(ctx, schema, options)
	if err != nil {
		t.Fatalf("create Mongo storage: %v", err)
	}
	defer func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		if err := storage.Database().Client().Disconnect(disconnectCtx); err != nil {
			t.Errorf("disconnect Mongo client: %v", err)
		}
	}()

	watcher, err := storage.Watch(ctx, &store.List[Message]{})
	if err != nil {
		t.Fatalf("watch messages: %v", err)
	}
	defer watcher.Stop()

	message := &Message{ObjectMeta: store.ObjectMeta{
		ID:   fmt.Sprintf("watch-%d", time.Now().UnixNano()),
		Name: "watch integration test",
	}}
	if err := storage.Create(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := storage.Delete(cleanupCtx, message); err != nil {
			t.Errorf("delete test message: %v", err)
		}
	}()

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

func mongoIntegrationOptions(t *testing.T) *MongoDBOptions {
	t.Helper()

	address := os.Getenv("COMMON_TEST_MONGO_ADDRESS")
	database := os.Getenv("COMMON_TEST_MONGO_DATABASE")
	if address == "" || database == "" {
		t.Skip("set COMMON_TEST_MONGO_ADDRESS and COMMON_TEST_MONGO_DATABASE to run")
	}

	options := NewDefaultMongoOptions(database)
	options.Address = address
	options.Username = os.Getenv("COMMON_TEST_MONGO_USERNAME")
	options.Password = os.Getenv("COMMON_TEST_MONGO_PASSWORD")
	options.ReplicaSet = os.Getenv("COMMON_TEST_MONGO_REPLICA_SET")
	if direct := os.Getenv("COMMON_TEST_MONGO_DIRECT"); direct != "" {
		parsed, err := strconv.ParseBool(direct)
		if err != nil {
			t.Fatalf("parse COMMON_TEST_MONGO_DIRECT: %v", err)
		}
		options.Direct = parsed
	}
	return options
}
