package mongo

import (
	"context"
	stderrors "errors"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

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
	_, newObjFunc, err := store.NewItemFuncFromList(obj)
	if err != nil {
		return nil, err
	}
	var watcher store.Watcher
	err = m.on(ctx, obj, func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
		filter = conditionsmatch(filter, SelectorToReqirements(options.LabelRequirements, options.FieldRequirements))
		if options.ID != "" {
			filter = append(filter, bson.E{Key: "id", Value: options.ID})
		}
		newwatcher, err := NewMongoWatcher(ctx, col, m.core.bsonOptions, m.core.bsonRegistry, newObjFunc, options, filter)
		if err != nil {
			return err
		}
		watcher = newwatcher
		return nil
	})
	return watcher, err
}

func toWatchFilter(filter bson.D) bson.D {
	ret := bson.D{
		bson.E{Key: "fullDocument", Value: bson.M{"$exists": true}},
		bson.E{Key: "operationType", Value: bson.M{"$in": bson.A{"insert", "update", "replace", "delete"}}},
	}
	// https://www.mongodb.com/docs/manual/reference/change-events/
	for _, f := range filter {
		f.Key = "fullDocument." + f.Key
		ret = append(ret, f)
	}
	return ret
}

var _ store.Watcher = &MongoWatcher{}

func NewMongoWatcher(ctx context.Context,
	col *mongo.Collection,
	bsonOptions *mongooptions.BSONOptions,
	bsonRegistry *bsoncodec.Registry,
	newobj func() store.Object,
	opts store.WatchOptions,
	filter bson.D,
) (*MongoWatcher, error) {
	watchCtx, cancel := context.WithCancel(ctx)
	var cur *mongo.Cursor
	if opts.SendInitialEvents {
		log.FromContext(watchCtx).Info("send initial events", "filter", filter)
		newcur, err := col.Find(watchCtx, filter)
		if err != nil {
			cancel()
			return nil, err
		}
		cur = newcur
	}
	stream, err := col.Watch(watchCtx,
		mongo.Pipeline{
			bson.D{{Key: "$match", Value: toWatchFilter(filter)}},
		},
		// check support full document on delete
		// https://www.mongodb.com/docs/manual/reference/change-events/delete/#document-pre--and-post-images
		mongooptions.ChangeStream().
			SetFullDocument(mongooptions.Required).
			SetFullDocumentBeforeChange(mongooptions.WhenAvailable),
	)
	if err != nil {
		if cur != nil {
			_ = cur.Close(watchCtx)
		}
		cancel()
		return nil, errors.NewInternalError(err)
	}
	w := &MongoWatcher{
		col:           col,
		bsonRegistry:  bsonRegistry,
		bsonOptions:   bsonOptions,
		newObjectFunc: newobj,
		results:       make(chan store.WatchEvent, 64),
		cancel:        cancel,
	}
	go w.run(watchCtx, cur, stream)
	return w, nil
}

type MongoWatcher struct {
	col           *mongo.Collection
	bsonRegistry  *bsoncodec.Registry
	bsonOptions   *mongooptions.BSONOptions
	newObjectFunc func() store.Object
	results       chan store.WatchEvent
	dropOnFull    bool
	cancel        context.CancelFunc
}

// Event implements Watcher.
func (w *MongoWatcher) Events() <-chan store.WatchEvent {
	return w.results
}

func (w *MongoWatcher) runlist(ctx context.Context, cur *mongo.Cursor) error {
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		item := w.newObjectFunc()
		if err := cur.Decode(item); err != nil {
			return errors.NewInternalError(err)
		}
		item.SetResource(w.col.Name())
		if !w.send(ctx, store.WatchEvent{Type: store.WatchEventCreate, Object: item}) {
			return ctx.Err()
		}
	}
	if err := cur.Err(); err != nil {
		return errors.NewInternalError(err)
	}
	if !w.send(ctx, store.WatchEvent{Type: store.WatchEventBookmark}) {
		return ctx.Err()
	}
	return nil
}

func (w *MongoWatcher) run(ctx context.Context, cur *mongo.Cursor, stream *mongo.ChangeStream) {
	defer close(w.results)
	defer w.Stop()
	if cur != nil {
		if err := w.runlist(ctx, cur); err != nil {
			if !stderrors.Is(err, context.Canceled) {
				w.send(ctx, store.WatchEvent{Error: err})
			}
			return
		}
	}
	type rawMongoEvent struct {
		OperationType            string   `json:"operationType"`
		FullDocument             bson.Raw `json:"fullDocument"`
		FullDocumentBeforeChange bson.Raw `json:"fullDocumentBeforeChange"`
		DocumentKey              bson.Raw `json:"documentKey"`
	}
	defer stream.Close(ctx)

	for stream.Next(ctx) {
		select {
		case <-ctx.Done():
			return
		default:
			raw := rawMongoEvent{}
			if err := stream.Decode(&raw); err != nil {
				w.send(ctx, store.WatchEvent{Error: errors.NewInternalError(err)})
				return
			}
			event := store.WatchEvent{}

			var old, new store.Object
			if olddoc := raw.FullDocumentBeforeChange; len(olddoc) > 0 {
				old = w.newObjectFunc()
				if err := bsonUnmarshal(w.bsonRegistry, olddoc, old); err != nil {
					w.send(ctx, store.WatchEvent{Error: errors.NewInternalError(err)})
					return
				}
				old.SetResource(w.col.Name())
			}
			if newdoc := raw.FullDocument; len(newdoc) > 0 {
				new = w.newObjectFunc()
				if err := bsonUnmarshal(w.bsonRegistry, newdoc, new); err != nil {
					w.send(ctx, store.WatchEvent{Error: errors.NewInternalError(err)})
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
			if !w.send(ctx, event) {
				return
			}
		}
	}
	if err := stream.Err(); err != nil {
		if stderrors.Is(err, context.Canceled) {
			return
		}
		w.send(ctx, store.WatchEvent{Error: errors.NewInternalError(err)})
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

func (w *MongoWatcher) send(ctx context.Context, e store.WatchEvent) bool {
	if w.dropOnFull {
		select {
		case w.results <- e:
			return true
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	select {
	case w.results <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

func (w *MongoWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}
