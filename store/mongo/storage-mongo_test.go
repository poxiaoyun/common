package mongo

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/store/storetest"
	testmongodb "xiaoshiai.cn/common/testkit/mongodb"
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

func (*TestObject) ResourceName() string {
	return "testobjects"
}

func TestMongoStorageCapabilities(t *testing.T) {
	capabilities := (&MongoStorage{}).Capabilities()
	if !capabilities.Watch {
		t.Fatal("Capabilities().Watch = false, want true")
	}
}

func TestMongoStorageIntegration(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	database := RequireIntegrationDatabase(t, uri)
	storage := NewIntegrationStorage(t, database, &TestObject{})

	objects := []*TestObject{
		{
			ObjectMeta: store.ObjectMeta{
				ID:   "platform",
				Name: "Platform",
				Labels: map[string]string{
					"example.com/team": "platform",
					"$owner":           "alice",
				},
				Annotations: map[string]string{
					"example.com/trace": "trace-1",
					"$reference":        "reference-1",
				},
			},
		},
		{
			ObjectMeta: store.ObjectMeta{
				ID:   "infrastructure",
				Name: "Infrastructure",
				Labels: map[string]string{
					"example.com/team": "infrastructure",
				},
			},
		},
		{
			ObjectMeta: store.ObjectMeta{
				ID:   "unlabeled",
				Name: "Unlabeled",
			},
		},
	}
	for _, object := range objects {
		if err := storage.Create(t.Context(), object); err != nil {
			t.Fatalf("Create(%q) error = %v", object.ID, err)
		}
	}

	stored := &TestObject{}
	if err := storage.Get(t.Context(), "platform", stored); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(stored.Labels, objects[0].Labels) {
		t.Fatalf("Get() labels = %#v, want %#v", stored.Labels, objects[0].Labels)
	}
	if !reflect.DeepEqual(stored.Annotations, objects[0].Annotations) {
		t.Fatalf("Get() annotations = %#v, want %#v", stored.Annotations, objects[0].Annotations)
	}

	tests := []struct {
		name        string
		requirement store.Requirement
		want        []string
	}{
		{
			name:        "equals literal dotted key",
			requirement: store.NewRequirement("example.com/team", store.Equals, "platform"),
			want:        []string{"platform"},
		},
		{
			name:        "not equals includes missing key",
			requirement: store.NewRequirement("example.com/team", store.NotEquals, "platform"),
			want:        []string{"infrastructure", "unlabeled"},
		},
		{
			name:        "in",
			requirement: store.NewRequirement("example.com/team", store.In, "platform"),
			want:        []string{"platform"},
		},
		{
			name:        "not in includes missing key",
			requirement: store.NewRequirement("example.com/team", store.NotIn, "platform"),
			want:        []string{"infrastructure", "unlabeled"},
		},
		{
			name:        "exists",
			requirement: store.NewRequirement("example.com/team", store.Exists),
			want:        []string{"infrastructure", "platform"},
		},
		{
			name:        "does not exist",
			requirement: store.NewRequirement("example.com/team", store.DoesNotExist),
			want:        []string{"unlabeled"},
		},
		{
			name:        "equals literal dollar key",
			requirement: store.NewRequirement("$owner", store.Equals, "alice"),
			want:        []string{"platform"},
		},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			list := &store.List[TestObject]{}
			if err := storage.List(
				t.Context(),
				list,
				store.WithLabelRequirements(current.requirement),
			); err != nil {
				t.Fatalf("List() error = %v", err)
			}
			got := make([]string, 0, len(list.Items))
			for _, item := range list.Items {
				got = append(got, item.ID)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, current.want) {
				t.Fatalf("List() IDs = %v, want %v", got, current.want)
			}
		})
	}
}

func TestStoreConformance(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	capabilities := (&MongoStorage{}).Capabilities()
	storetest.Run(t, storetest.Fixture{
		Capabilities: capabilities,
		New: func(t testing.TB, schema *store.Schema) (store.Store, error) {
			database := RequireIntegrationDatabase(t, uri)
			core := &MongoStorageCore{
				schema:       schema,
				db:           database,
				bsonRegistry: GlobalBsonRegistry,
				bsonOptions: &mongooptions.BSONOptions{
					UseJSONStructTags: true,
					OmitZeroStruct:    true,
				},
				collections:    map[string]*mongo.Collection{},
				collectionLock: sync.RWMutex{},
				logger:         log.FromContext(t.Context()).WithName("mongo-storage"),
			}
			if err := core.initCollections(t.Context()); err != nil {
				return nil, err
			}
			return &MongoStorage{core: core}, nil
		},
	})
}

func TestMongoStorageBatchPatchAdvancesResourceVersion(t *testing.T) {
	uri := testmongodb.RequireURI(t)
	database := RequireIntegrationDatabase(t, uri)
	storage := NewIntegrationStorage(t, database, &TestObject{})

	object := &TestObject{ObjectMeta: store.ObjectMeta{ID: "batch-version"}}
	if err := storage.Create(t.Context(), object); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	previous := object.ResourceVersion

	list := &store.List[TestObject]{}
	if err := storage.PatchBatch(
		t.Context(),
		list,
		store.RawPatchBatch(store.PatchTypeMergePatch, []byte(`{"description":"patched"}`)),
		store.WithPatchBatchFieldRequirements(store.NewRequirement("id", store.Equals, object.ID)),
	); err != nil {
		t.Fatalf("PatchBatch() error = %v", err)
	}

	got := &TestObject{}
	if err := storage.Get(t.Context(), object.ID, got); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ResourceVersion <= previous || got.Description != "patched" {
		t.Fatalf("PatchBatch() object = %#v, want description patched and version > %d", got, previous)
	}
}

// RequireIntegrationDatabase creates an isolated database with the store's BSON codecs.
func RequireIntegrationDatabase(t testing.TB, uri string) *mongo.Database {
	t.Helper()
	return testmongodb.RequireDatabase(
		t,
		uri,
		mongooptions.Client().
			SetBSONOptions(&mongooptions.BSONOptions{
				UseJSONStructTags: true,
				OmitZeroStruct:    true,
			}).
			SetRegistry(GlobalBsonRegistry),
	)
}

// NewIntegrationStorage creates a store over a testkit-managed database.
func NewIntegrationStorage(t testing.TB, database *mongo.Database, objects ...store.Object) *MongoStorage {
	t.Helper()
	schema := store.NewSchema()
	for _, object := range objects {
		if err := schema.Register(object, store.ResourceSchema{}); err != nil {
			t.Fatalf("register schema: %v", err)
		}
	}
	core := &MongoStorageCore{
		schema:       schema,
		db:           database,
		bsonRegistry: GlobalBsonRegistry,
		bsonOptions: &mongooptions.BSONOptions{
			UseJSONStructTags: true,
			OmitZeroStruct:    true,
		},
		collections:    map[string]*mongo.Collection{},
		collectionLock: sync.RWMutex{},
		logger:         log.FromContext(t.Context()).WithName("mongo-storage"),
	}
	if err := core.initCollections(t.Context()); err != nil {
		t.Fatalf("initialize MongoDB collections: %v", err)
	}
	return &MongoStorage{core: core}
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

func TestSelectorMatchUsesLiteralLabelKeys(t *testing.T) {
	tests := []struct {
		name        string
		requirement store.Requirement
		want        bson.D
	}{
		{
			name:        "equals",
			requirement: store.NewRequirement("example.com/team", store.Equals, "platform"),
			want:        labelExpression("$labels", "example.com/team", "$eq", "platform"),
		},
		{
			name:        "double equals",
			requirement: store.NewRequirement("example.com/team", store.DoubleEquals, "platform"),
			want:        labelExpression("$labels", "example.com/team", "$eq", "platform"),
		},
		{
			name:        "not equals",
			requirement: store.NewRequirement("example.com/team", store.NotEquals, "platform"),
			want:        labelExpression("$labels", "example.com/team", "$ne", "platform"),
		},
		{
			name:        "in",
			requirement: store.NewRequirement("example.com/team", store.In, "platform", "infrastructure"),
			want:        labelInExpression("$labels", "example.com/team", false, "platform", "infrastructure"),
		},
		{
			name:        "not in",
			requirement: store.NewRequirement("example.com/team", store.NotIn, "platform", "infrastructure"),
			want:        labelInExpression("$labels", "example.com/team", true, "platform", "infrastructure"),
		},
		{
			name:        "exists",
			requirement: store.NewRequirement("example.com/team", store.Exists),
			want:        labelExpression("$labels", "example.com/team", "$ne", nil),
		},
		{
			name:        "does not exist",
			requirement: store.NewRequirement("example.com/team", store.DoesNotExist),
			want:        labelExpression("$labels", "example.com/team", "$eq", nil),
		},
		{
			name:        "dollar prefix",
			requirement: store.NewRequirement("$team", store.Equals, "platform"),
			want:        labelExpression("$labels", "$team", "$eq", "platform"),
		},
		{
			name:        "ordinary key",
			requirement: store.NewRequirement("app", store.Equals, "api"),
			want:        labelExpression("$labels", "app", "$eq", "api"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConditionsMatch(
				bson.D{},
				store.Requirements{tt.requirement},
				nil,
				"",
			)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("selector match = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSelectorMatchUsesAndWithFieldRequirements(t *testing.T) {
	labels := store.Requirements{
		{
			Key:      "example.com/team",
			Operator: store.Equals,
			Values:   []any{"platform"},
		},
		{
			Key:      "app",
			Operator: store.Exists,
		},
	}
	fields := store.Requirements{
		{
			Key:      "status.phase",
			Operator: store.Equals,
			Values:   []any{"active"},
		},
	}
	want := bson.D{
		{Key: "status.phase", Value: "active"},
		{Key: "$expr", Value: bson.D{
			{Key: "$and", Value: bson.A{
				labelPredicate("$labels", "example.com/team", "$eq", "platform"),
				labelPredicate("$labels", "app", "$ne", nil),
			}},
		}},
	}

	got := ConditionsMatch(bson.D{}, labels, fields, "")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selector match = %#v, want %#v", got, want)
	}
}

func labelExpression(input, key, operator string, value any) bson.D {
	return bson.D{
		{Key: "$expr", Value: bson.D{
			{Key: "$and", Value: bson.A{
				labelPredicate(input, key, operator, value),
			}},
		}},
	}
}

func labelPredicate(input, key, operator string, value any) bson.D {
	right := any(bson.D{{Key: "$literal", Value: value}})
	if value == nil {
		right = nil
	}
	return bson.D{
		{Key: operator, Value: bson.A{
			literalLabelValue(input, key),
			right,
		}},
	}
}

func labelInExpression(input, key string, negate bool, values ...any) bson.D {
	predicate := bson.D{
		{Key: "$in", Value: bson.A{
			literalLabelValue(input, key),
			bson.D{{Key: "$literal", Value: bson.A(values)}},
		}},
	}
	if negate {
		predicate = bson.D{{Key: "$not", Value: bson.A{predicate}}}
	}
	return bson.D{
		{Key: "$expr", Value: bson.D{
			{Key: "$and", Value: bson.A{predicate}},
		}},
	}
}

func literalLabelValue(input, key string) bson.D {
	return bson.D{
		{Key: "$ifNull", Value: bson.A{
			bson.D{
				{Key: "$getField", Value: bson.D{
					{Key: "field", Value: bson.D{{Key: "$literal", Value: key}}},
					{Key: "input", Value: input},
				}},
			},
			nil,
		}},
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
