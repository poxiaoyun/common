package mongo

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"xiaoshiai.cn/common/store"
)

type TestObject struct {
	store.ObjectMeta `json:",inline"`
	Status           TestObjectStatus `json:"status"`
}

type TestObjectStatus struct {
	Val   string   `json:"val"`
	Int   int      `json:"int"`
	Slice []string `json:"slice"`
}

func TestMergePatchToBsonUpdate(t *testing.T) {
	type args struct {
		data map[string]any
	}
	tests := []struct {
		name    string
		args    args
		want    bson.D
		wantErr bool
	}{
		{
			name: "default",
			args: args{
				data: map[string]any{
					"alias": "test",
					"status": map[string]any{
						"val": "test",
					},
				},
			},
			want: bson.D{
				{Key: "$set", Value: bson.D{
					{Key: "alias", Value: "test"},
					{Key: "status.val", Value: "test"},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergePatchToBsonUpdate(tt.args.data, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("MergePatchToBsonUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(normalizeBSONDocument(got), normalizeBSONDocument(tt.want)) {
				t.Errorf("MergePatchToBsonUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func normalizeBSONDocument(doc bson.D) bson.D {
	normalized := make(bson.D, len(doc))
	for i, elem := range doc {
		normalized[i].Key = elem.Key
		switch value := elem.Value.(type) {
		case bson.D:
			normalized[i].Value = normalizeBSONDocument(value)
		case bson.A:
			items := make(bson.A, len(value))
			for i, item := range value {
				if nested, ok := item.(bson.D); ok {
					items[i] = normalizeBSONDocument(nested)
				} else {
					items[i] = item
				}
			}
			normalized[i].Value = items
		default:
			normalized[i].Value = value
		}
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Key < normalized[j].Key
	})
	return normalized
}

func TestMongoWatcherStopDoesNotCloseProducerChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &MongoWatcher{
		results: make(chan store.WatchEvent, 1),
		cancel:  cancel,
	}
	watcher.Stop()
	watcher.Stop()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("Stop() did not cancel the watcher context")
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("send after Stop() panicked: %v", recovered)
		}
	}()
	watcher.results <- store.WatchEvent{Type: store.WatchEventBookmark}
}
