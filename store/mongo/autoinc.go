package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"k8s.io/utils/ptr"
)

const CounterCollection = "counters"

// Counter is a counter for auto increment.
func GetCounter(ctx context.Context, db *mongo.Database, name string) (uint64, error) {
	collection := db.Collection(CounterCollection)
	filter := bson.M{"_id": name}
	update := bson.M{"$inc": bson.M{"seq": 1}}
	after := options.After
	opt := options.FindOneAndUpdateOptions{ReturnDocument: &after, Upsert: ptr.To(true)}
	var result struct {
		Seq uint64 `bson:"seq"`
	}
	err := collection.FindOneAndUpdate(ctx, filter, update, &opt).Decode(&result)
	if err != nil {
		return 0, err
	}
	return result.Seq, nil
}
