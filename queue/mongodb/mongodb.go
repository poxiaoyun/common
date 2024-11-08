package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/queue"
)

type MongoQueueOptions struct {
	CheckInterval time.Duration
}

func NewMongoDBQueue(col *mongo.Collection, options *MongoQueueOptions) *MongoQueue {
	if options.CheckInterval == 0 {
		options.CheckInterval = 1 * time.Minute
	}
	return &MongoQueue{
		col:     col,
		options: options,
	}
}

type MongoQueue struct {
	col     *mongo.Collection
	options *MongoQueueOptions

	results chan *QueueItem
}

type QueueItem struct {
	ID       primitive.ObjectID `json:"_id,omitempty" bson:"_id"`
	Payload  string             `json:"payload,omitempty" bson:"payload"`
	Statuses []QueueItemStatus  `json:"statuses,omitempty" bson:"statuses"`
}

type QueueItemStatus struct {
	Status    string     `json:"status,omitempty" bson:"status"`
	Timestamp time.Time  `json:"timestamp,omitempty" bson:"timestamp"`
	NextRetry *time.Time `json:"nextRetry,omitempty" bson:"nextRetry"`
	Message   string     `json:"message,omitempty" bson:"message"`
}

const (
	QueueItemStatusEnqueued   = "Enqueued"
	QueueItemStatusProcessing = "Processing"
	QueueItemStatusDone       = "Done"
	QueueItemStatusFailed     = "Failed"
)

func (q *MongoQueue) Enqueue(ctx context.Context, data string, options queue.EnqueueOptions) error {
	result, err := q.col.InsertOne(ctx, QueueItem{
		ID:       primitive.NewObjectID(),
		Payload:  data,
		Statuses: []QueueItemStatus{{Status: QueueItemStatusEnqueued, Timestamp: time.Now(), NextRetry: nil}},
	})
	if err != nil {
		return err
	}
	_ = result.InsertedID
	return nil
}

func (q *MongoQueue) Consume(ctx context.Context, fn queue.QueueHandleFunc, options queue.ConsumeOptions) error {
	if err := q.initConsumes(ctx, fn); err != nil {
		return err
	}
	changed := make(chan primitive.ObjectID, 1)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return q.checkStreamChange(ctx, changed)
	})
	eg.Go(func() error {
		tim := time.NewTimer(q.options.CheckInterval)
		for {
			select {
			case <-ctx.Done():
				return nil
			case id := <-changed:
				ok, err := q.tryProcess(ctx, id, fn)
				if err != nil {
					return err
				}
				if !ok {
					// Reset timer
					tim.Reset(q.options.CheckInterval)
				} else {
					tim.Reset(time.Second)
				}
			case <-tim.C:
				ok, err := q.tryProcess(ctx, primitive.NilObjectID, fn)
				if err != nil {
					return err
				}
				if !ok {
					// Reset timer
					tim.Reset(q.options.CheckInterval)
				} else {
					tim.Reset(time.Second)
				}
			}
		}
	})
	return eg.Wait()
}

func (q *MongoQueue) initConsumes(ctx context.Context, hand queue.QueueHandleFunc) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			item, ok, err := q.dequeue(ctx)
			if err != nil {
				return err
			}
			// No more items
			if !ok {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			default:
				return q.process(ctx, item, hand)
			}
		}
	}
}

func (q *MongoQueue) process(ctx context.Context, item *QueueItem, hand queue.QueueHandleFunc) error {
	log := log.FromContext(ctx)
	if err := hand(ctx, item.ID.Hex(), item.Payload); err != nil {
		log.Error(err, "handle queue item failed", "id", item.ID.Hex())
		if err := q.nack(ctx, item.ID, err.Error()); err != nil {
			return err
		}
		return nil
	}
	if err := q.ack(ctx, item.ID); err != nil {
		return err
	}
	return nil
}

func (q *MongoQueue) tryProcess(ctx context.Context, id primitive.ObjectID, hand queue.QueueHandleFunc) (bool, error) {
	item, ok, err := q.dequeueWithID(ctx, id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	select {
	case <-ctx.Done():
		return false, nil
	default:
		if err := q.process(ctx, item, hand); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (q *MongoQueue) dequeue(ctx context.Context) (*QueueItem, bool, error) {
	return q.dequeueWithID(ctx, primitive.NilObjectID)
}

func (q *MongoQueue) dequeueWithID(ctx context.Context, id primitive.ObjectID) (*QueueItem, bool, error) {
	conds := []bson.M{
		{"statuses.0.status": QueueItemStatusEnqueued},
		{"statuses.0.nextRetry": nil},
	}
	if !id.IsZero() {
		conds = append(conds, bson.M{"_id": id})
	}
	result := q.col.FindOneAndUpdate(ctx,
		bson.M{"$and": conds},
		bson.M{
			"$push": bson.M{
				"statuses": bson.M{
					"$each": []QueueItemStatus{
						{
							Status:    QueueItemStatusProcessing,
							Timestamp: time.Now(),
							NextRetry: nil,
						},
					},
					"$position": 0,
				},
			},
		})
	var item QueueItem
	if err := result.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, false, nil
		}
		return nil, false, err
	}
	if err := result.Decode(&item); err != nil {
		return nil, false, err
	}
	if len(item.Statuses) != 0 && item.Statuses[0].Status == QueueItemStatusEnqueued {
		if nexttry := item.Statuses[0].NextRetry; nexttry != nil {
			if nexttry.After(time.Now()) {
				return nil, false, nil
			}
		}
	}
	return &item, true, nil
}

func (q *MongoQueue) ack(ctx context.Context, id primitive.ObjectID) error {
	_, err := q.col.UpdateByID(ctx, id, bson.M{
		"$push": bson.M{
			"statuses": bson.M{
				"$each": []QueueItemStatus{
					{
						Status:    QueueItemStatusDone,
						Timestamp: time.Now(),
						NextRetry: nil,
					},
				},
				"$position": 0,
			},
		},
	})
	return err
}

func (q *MongoQueue) nack(ctx context.Context, id primitive.ObjectID, msg string) error {
	_, err := q.col.UpdateByID(ctx, id, bson.M{
		"$push": bson.M{
			"statuses": bson.M{
				"$each": []QueueItemStatus{
					{
						Status:    QueueItemStatusFailed,
						Timestamp: time.Now(),
						NextRetry: nil,
						Message:   msg,
					},
				},
				"$position": 0,
			},
		},
	})
	return err
}

func (q *MongoQueue) checkStreamChange(ctx context.Context, changed chan primitive.ObjectID) error {
	stream, err := q.col.Watch(ctx, bson.A{}, options.ChangeStream())
	if err != nil {
		return errors.NewInternalError(err)
	}
	type DocumentKey struct {
		ID primitive.ObjectID `json:"_id" bson:"_id"`
	}
	type rawMongoEvent struct {
		OperationType            string      `json:"operationType"`
		FullDocument             bson.Raw    `json:"fullDocument"`
		FullDocumentBeforeChange bson.Raw    `json:"fullDocumentBeforeChange"`
		DocumentKey              DocumentKey `json:"documentKey"`
	}

	for stream.Next(ctx) {
		select {
		case <-ctx.Done():
			return nil
		default:
			raw := rawMongoEvent{}
			if err := stream.Decode(&raw); err != nil {
				return errors.NewInternalError(err)
			}
			if raw.OperationType == "insert" {
				changed <- raw.DocumentKey.ID
			}
		}
	}
	if err := stream.Err(); err != nil {
		return errors.NewInternalError(err)
	}
	return nil
}
