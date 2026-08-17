// Package mongodb provides a MongoDB-backed sequence allocator.
package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Allocator persists sequences in a MongoDB collection. The key is stored as
// the document _id, whose built-in unique index makes allocation atomic.
type Allocator struct {
	collection *mongo.Collection
}

// New creates an allocator backed by collection.
func New(collection *mongo.Collection) *Allocator {
	return &Allocator{collection: collection}
}

// Next implements sequence.Allocator.
func (a *Allocator) Next(ctx context.Context, key string) (uint64, error) {
	result := struct {
		Value uint64 `bson:"value"`
	}{}
	err := a.collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": key},
		bson.M{"$inc": bson.M{"value": 1}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&result)
	return result.Value, err
}
