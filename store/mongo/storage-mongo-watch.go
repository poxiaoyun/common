package mongo

import (
	"context"
	stderrors "errors"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

type Watcher interface {
	Close() error
	Events() <-chan WatchEvent
}
type WatchEventType string

const (
	WatchEventAdd    WatchEventType = "add"
	WatchEventUpdate WatchEventType = "update"
	WatchEventDelete WatchEventType = "delete"
)

type WatchEvent struct {
	Type  WatchEventType
	Error error
	Value store.Object
}

func NewObject[T any](t reflect.Type) T {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		return reflect.New(t).Interface().(T)
	}
	return reflect.New(t).Elem().Interface().(T)
}

// Watch implements Storage.
func (m *MongoStorage) Watch(ctx context.Context, obj store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	options := store.WatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	itemsPointer, err := store.GetItemsPtr(obj)
	if err != nil {
		return nil, err
	}
	itemType := reflect.TypeOf(itemsPointer).Elem().Elem()
	newObjFunc := func() store.Object {
		return NewObject[store.Object](itemType)
	}
	var watcher store.Watcher
	err = m.on(ctx, obj, func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
		filter = conditionsmatch(filter, FieldsSelectorToReqirements(options.FieldSelector))
		newwatcher, err := NewMongoWatcher(ctx, col, m.core.bsonOptions, m.core.bsonRegistry, newObjFunc, filter)
		if err != nil {
			return err
		}
		watcher = newwatcher
		return nil
	})
	return watcher, err
}

var _ store.Watcher = &MongoWatcher{}

func NewMongoWatcher(ctx context.Context,
	col *mongo.Collection,
	bsonOptions *mongooptions.BSONOptions,
	bsonRegistry *bsoncodec.Registry,
	newobj func() store.Object,
	filter any,
) (*MongoWatcher, error) {
	// check support full document on delete
	// https://www.mongodb.com/docs/manual/reference/change-events/delete/#document-pre--and-post-images
	stream, err := col.Watch(ctx,
		mongo.Pipeline{
			bson.D{{Key: "$match", Value: filter}},
		},
		options.ChangeStream().
			SetFullDocument(options.Required).
			SetFullDocumentBeforeChange(options.Required))
	if err != nil {
		return nil, errors.NewInternalError(err)
	}

	w := &MongoWatcher{
		col:           col,
		bsonRegistry:  bsonRegistry,
		bsonOptions:   bsonOptions,
		newObjectFunc: newobj,
		stream:        stream,
		closed:        false,
		results:       make(chan store.WatchEvent, 64),
	}
	go w.run(ctx)
	return w, nil
}

type MongoWatcher struct {
	col          *mongo.Collection
	bsonRegistry *bsoncodec.Registry
	bsonOptions  *mongooptions.BSONOptions

	stream        *mongo.ChangeStream
	closed        bool
	newObjectFunc func() store.Object
	results       chan store.WatchEvent
	dropOnFull    bool
}

// Event implements Watcher.
func (w *MongoWatcher) Events() <-chan store.WatchEvent {
	return w.results
}

func (w *MongoWatcher) run(ctx context.Context) {
	type rawMongoEvent struct {
		OperationType            string   `json:"operationType"`
		FullDocument             bson.Raw `json:"fullDocument"`
		FullDocumentBeforeChange bson.Raw `json:"fullDocumentBeforeChange"`
		DocumentKey              bson.Raw `json:"documentKey"`
	}
	for w.stream.Next(ctx) {
		select {
		case <-ctx.Done():
			return
		default:
			raw := rawMongoEvent{}
			if err := w.stream.Decode(&raw); err != nil {
				w.results <- store.WatchEvent{Error: errors.NewInternalError(err)}
				return
			}
			event := store.WatchEvent{}

			var old, new store.Object
			if olddoc := raw.FullDocumentBeforeChange; len(olddoc) > 0 {
				old = w.newObjectFunc()
				if err := bsonUnmarshal(w.bsonRegistry, olddoc, old); err != nil {
					w.send(store.WatchEvent{Error: errors.NewInternalError(err)})
					return
				}
				old.SetResource(w.col.Name())
			}
			if newdoc := raw.FullDocument; len(newdoc) > 0 {
				new = w.newObjectFunc()
				if err := bsonUnmarshal(w.bsonRegistry, newdoc, new); err != nil {
					w.send(store.WatchEvent{Error: errors.NewInternalError(err)})
					return
				}
				new.SetResource(w.col.Name())
			}
			switch raw.OperationType {
			case "insert":
				event.Type = store.WatchEventCreate
				event.Object = new
			case "update", "replace":
				event.Type = store.WatchEventUpdate
				event.Object = new
			case "delete":
				event.Type = store.WatchEventDelete
				event.Object = old
			}
			w.send(event)
		}
	}
	if err := w.stream.Err(); err != nil {
		if stderrors.Is(err, context.Canceled) {
			return
		}
		w.send(store.WatchEvent{Error: errors.NewInternalError(err)})
	}
}

func bsonUnmarshal(bsoncodec *bsoncodec.Registry, data []byte, obj store.Object) error {
	dec, err := bson.NewDecoder(bsonrw.NewBSONDocumentReader(data))
	if err != nil {
		return err
	}
	dec.SetRegistry(bsoncodec)
	dec.ZeroStructs()
	dec.UseJSONStructTags()
	return dec.Decode(obj)
}

func (w *MongoWatcher) send(e store.WatchEvent) {
	if w.closed {
		return
	}
	if w.dropOnFull {
		select {
		case w.results <- e:
		default:
		}
	} else {
		w.results <- e
	}
}

func (w *MongoWatcher) Stop() {
	if w.closed {
		return
	}
	close(w.results)
	w.stream.Close(nil)
}
