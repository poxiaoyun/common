// Package mongodb provides a persistent event bus backed by MongoDB 5.0 or
// newer. It requires a replica set or sharded cluster because it uses MongoDB
// cluster timestamps to order consumer activation and event acceptance.
//
// Consumption state is embedded in each event document and keyed only by
// ConsumerID. This keeps claiming, retrying, and acknowledging a delivery
// atomic at the single-document level. The adapter is intended for a bounded
// number of consumers and moderate event retention; every retained event grows
// by one small state entry for each consumer that attempts it.
package mongodb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"
	"xiaoshiai.cn/common/eventbus"
)

const (
	defaultMaxConcurrency = 1
	defaultMaxAttempts    = 3
	defaultPollInterval   = time.Second
	defaultLeaseDuration  = 30 * time.Second
	leaseExpiredError     = "delivery lease expired"
)

// Options configures a Bus. Zero values select defaults.
type Options struct {
	PollInterval  time.Duration
	LeaseDuration time.Duration
}

// New creates a MongoDB event bus and ensures the event indexes exist. Events
// and consumers must be different collections in the same MongoDB deployment.
func New(
	ctx context.Context,
	events *mongo.Collection,
	consumers *mongo.Collection,
	options Options,
) (*Bus, error) {
	if options.PollInterval < 0 || options.LeaseDuration < 0 {
		return nil, eventbus.ErrInvalidArgument
	}
	if options.PollInterval == 0 {
		options.PollInterval = defaultPollInterval
	}
	if options.LeaseDuration == 0 {
		options.LeaseDuration = defaultLeaseDuration
	}
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "idempotencyKey", Value: 1},
			},
			Options: mongooptions.Index().
				SetName("eventbus_idempotency").
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: "idempotencyKey", Value: bson.D{
						{Key: "$exists", Value: true},
					}},
				}),
		},
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "acceptedAt", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: mongooptions.Index().SetName("eventbus_consume"),
		},
	}
	if _, err := events.Indexes().CreateMany(ctx, indexes); err != nil {
		return nil, fmt.Errorf("create eventbus indexes: %w", err)
	}
	return &Bus{
		events:    events,
		consumers: consumers,
		options:   options,
	}, nil
}

var (
	_ eventbus.Publisher  = (*Bus)(nil)
	_ eventbus.Subscriber = (*Bus)(nil)
)

// Bus stores immutable event fields and per-consumer delivery state in the
// events collection. Consumer activation is stored separately so a new
// ConsumerID does not replay events accepted before its first Subscribe call.
type Bus struct {
	events    *mongo.Collection
	consumers *mongo.Collection
	options   Options
}

type eventDocument struct {
	ID             string                 `bson:"_id"`
	Type           string                 `bson:"type"`
	Source         string                 `bson:"source"`
	Subject        string                 `bson:"subject"`
	Time           time.Time              `bson:"time"`
	AcceptedAt     primitive.Timestamp    `bson:"acceptedAt"`
	Data           []byte                 `bson:"data"`
	Metadata       map[string]string      `bson:"metadata"`
	IdempotencyKey string                 `bson:"idempotencyKey,omitempty"`
	Consumptions   map[string]consumption `bson:"consumptions"`
}

type consumerDocument struct {
	Key         string              `bson:"_id"`
	ConsumerID  string              `bson:"consumerId"`
	ActivatedAt primitive.Timestamp `bson:"activatedAt"`
	Ephemeral   bool                `bson:"ephemeral"`
}

type consumptionState string

const (
	statePending consumptionState = "Pending"
	stateRunning consumptionState = "Running"
	stateAcked   consumptionState = "Acked"
	stateDead    consumptionState = "Dead"
)

type consumption struct {
	State          consumptionState `bson:"state"`
	Attempt        int              `bson:"attempt"`
	NotBefore      time.Time        `bson:"notBefore"`
	LastError      string           `bson:"lastError"`
	CompletionTime *time.Time       `bson:"completionTime,omitempty"`
	Lease          *lease           `bson:"lease,omitempty"`
}

type lease struct {
	Token       string    `bson:"token"`
	Deadline    time.Time `bson:"deadline"`
	MaxAttempts int       `bson:"maxAttempts"`
}

// Publish stores one immutable event document. Repeating an accepted event ID
// or IdempotencyKey with an equivalent event returns the original event ID.
func (b *Bus) Publish(
	ctx context.Context,
	event eventbus.Event,
	options eventbus.PublishOptions,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := eventbus.ValidateEventType(event.Type); err != nil {
		return "", err
	}
	original := cloneEvent(event)
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	doc := eventDocument{
		ID:             event.ID,
		Type:           event.Type,
		Source:         event.Source,
		Subject:        event.Subject,
		Time:           mongoTime(event.Time),
		Data:           bytes.Clone(event.Data),
		Metadata:       maps.Clone(event.Metadata),
		IdempotencyKey: options.IdempotencyKey,
		Consumptions:   make(map[string]consumption),
	}

	existing, err := b.insertEvent(ctx, doc)
	if err != nil {
		return "", err
	}
	if !equivalentEvent(existing, original, options.IdempotencyKey) {
		return "", eventbus.ErrConflict
	}
	return existing.ID, nil
}

func (b *Bus) insertEvent(ctx context.Context, doc eventDocument) (eventDocument, error) {
	pipeline := insertWithClusterTime(doc, "acceptedAt")
	findOptions := mongooptions.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(mongooptions.After)
	var stored eventDocument
	err := b.events.FindOneAndUpdate(ctx, bson.D{
		{Key: "_id", Value: doc.ID},
	}, pipeline, findOptions).Decode(&stored)
	if err == nil {
		return stored, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return eventDocument{}, fmt.Errorf("publish event: %w", err)
	}
	filter := bson.D{
		{Key: "_id", Value: doc.ID},
	}
	if doc.IdempotencyKey != "" {
		filter = bson.D{
			{Key: "idempotencyKey", Value: doc.IdempotencyKey},
		}
	}
	if err := b.events.FindOne(ctx, filter).Decode(&stored); err != nil {
		return eventDocument{}, fmt.Errorf("get conflicting event: %w", err)
	}
	return stored, nil
}

// Subscribe consumes events for one ConsumerID until ctx is canceled. Pattern
// matching selects eligible handlers but does not create independent progress:
// all Subscribe calls using the same non-empty ConsumerID update the same map
// entry in each event document.
func (b *Bus) Subscribe(
	ctx context.Context,
	eventTypePattern string,
	handler eventbus.Handler,
	options eventbus.SubscribeOptions,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := eventbus.CompileEventTypePattern(eventTypePattern); err != nil {
		return err
	}
	if handler == nil || options.MaxConcurrency < 0 || options.MaxAttempts < 0 || options.Timeout < 0 {
		return eventbus.ErrInvalidArgument
	}
	if options.MaxConcurrency == 0 {
		options.MaxConcurrency = defaultMaxConcurrency
	}
	if options.MaxAttempts == 0 {
		options.MaxAttempts = defaultMaxAttempts
	}

	consumer, err := b.ensureConsumer(ctx, options.ConsumerID)
	if err != nil {
		return err
	}
	if err := b.recoverExpired(ctx, consumer.Key, mongoTime(time.Now())); err != nil {
		return err
	}

	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return b.recoverLoop(runCtx, consumer.Key)
	})
	for range options.MaxConcurrency {
		group.Go(func() error {
			return b.workerLoop(runCtx, consumer, eventTypePattern, handler, options)
		})
	}
	runErr := group.Wait()
	if ctx.Err() != nil && errors.Is(runErr, ctx.Err()) {
		runErr = nil
	}
	if consumer.Ephemeral {
		cleanupContext := context.WithoutCancel(ctx)
		if _, err := b.consumers.DeleteOne(cleanupContext, bson.D{
			{Key: "_id", Value: consumer.Key},
		}); runErr == nil && err != nil {
			runErr = fmt.Errorf("delete ephemeral event consumer: %w", err)
		}
	}
	return runErr
}

func (b *Bus) ensureConsumer(ctx context.Context, id string) (consumerDocument, error) {
	ephemeral := id == ""
	identity := id
	if ephemeral {
		identity = uuid.NewString()
	}
	doc := consumerDocument{
		Key:        consumerKey(identity),
		ConsumerID: id,
		Ephemeral:  ephemeral,
	}
	pipeline := insertWithClusterTime(doc, "activatedAt")
	findOptions := mongooptions.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(mongooptions.After)
	var stored consumerDocument
	err := b.consumers.FindOneAndUpdate(ctx, bson.D{
		{Key: "_id", Value: doc.Key},
	}, pipeline, findOptions).Decode(&stored)
	if err == nil {
		return stored, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return consumerDocument{}, fmt.Errorf("register event consumer: %w", err)
	}
	if err := b.consumers.FindOne(ctx, bson.D{
		{Key: "_id", Value: doc.Key},
	}).Decode(&stored); err != nil {
		return consumerDocument{}, fmt.Errorf("get event consumer: %w", err)
	}
	return stored, nil
}

func (b *Bus) workerLoop(
	ctx context.Context,
	consumer consumerDocument,
	eventTypePattern string,
	handler eventbus.Handler,
	options eventbus.SubscribeOptions,
) error {
	ticker := time.NewTicker(b.options.PollInterval)
	defer ticker.Stop()
	for {
		claimed, err := b.claim(ctx, consumer, eventTypePattern, options.MaxAttempts, mongoTime(time.Now()))
		if err != nil {
			return err
		}
		if claimed != nil {
			if err := b.execute(ctx, consumer.Key, claimed, handler, options); err != nil {
				return err
			}
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

type claimedEvent struct {
	token   string
	event   eventbus.Event
	attempt int
}

func (b *Bus) claim(
	ctx context.Context,
	consumer consumerDocument,
	eventTypePattern string,
	maxAttempts int,
	now time.Time,
) (*claimedEvent, error) {
	path := consumptionPath(consumer.Key)
	token := uuid.NewString()
	deadline := mongoTime(now.Add(b.options.LeaseDuration))
	filter := bson.D{
		{Key: "acceptedAt", Value: bson.D{
			{Key: "$gt", Value: consumer.ActivatedAt},
		}},
		eventTypeFilter(eventTypePattern),
		{Key: "$or", Value: bson.A{
			bson.D{
				{Key: path, Value: bson.D{
					{Key: "$exists", Value: false},
				}},
			},
			bson.D{
				{Key: path + ".state", Value: statePending},
				{Key: path + ".notBefore", Value: bson.D{
					{Key: "$lte", Value: now},
				}},
			},
		}},
	}
	update := mongo.Pipeline{
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: path, Value: bson.D{
					{Key: "state", Value: stateRunning},
					{Key: "attempt", Value: bson.D{
						{Key: "$add", Value: bson.A{
							bson.D{
								{Key: "$ifNull", Value: bson.A{"$" + path + ".attempt", 0}},
							},
							1,
						}},
					}},
					{Key: "notBefore", Value: now},
					{Key: "lastError", Value: ""},
					{Key: "lease", Value: lease{
						Token:       token,
						Deadline:    deadline,
						MaxAttempts: maxAttempts,
					}},
				}},
			}},
		},
	}
	findOptions := mongooptions.FindOneAndUpdate().
		SetReturnDocument(mongooptions.After).
		SetSort(bson.D{
			{Key: "acceptedAt", Value: 1},
			{Key: "_id", Value: 1},
		})
	var doc eventDocument
	if err := b.events.FindOneAndUpdate(ctx, filter, update, findOptions).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim event delivery: %w", err)
	}
	state := doc.Consumptions[consumer.Key]
	return &claimedEvent{
		token:   token,
		event:   doc.event(),
		attempt: state.Attempt,
	}, nil
}

type renewal struct {
	lost bool
	err  error
}

func (b *Bus) execute(
	ctx context.Context,
	consumerKey string,
	claimed *claimedEvent,
	handler eventbus.Handler,
	options eventbus.SubscribeOptions,
) error {
	handlerContext, cancel := context.WithCancel(ctx)
	if options.Timeout > 0 {
		handlerContext, cancel = context.WithTimeout(ctx, options.Timeout)
	}
	done := make(chan struct{})
	renewed := make(chan renewal, 1)
	go func() {
		renewed <- b.renew(handlerContext, cancel, consumerKey, claimed, done)
	}()
	handlerErr := handler.Handle(handlerContext, cloneEvent(claimed.event))
	close(done)
	result := <-renewed
	cancel()
	if result.err != nil {
		return result.err
	}
	if ctx.Err() != nil || result.lost {
		return nil
	}
	return b.finish(ctx, consumerKey, claimed, handlerErr, options.MaxAttempts, mongoTime(time.Now()))
}

func (b *Bus) renew(
	ctx context.Context,
	cancel context.CancelFunc,
	consumerKey string,
	claimed *claimedEvent,
	done <-chan struct{},
) renewal {
	interval := max(b.options.LeaseDuration/3, time.Nanosecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	path := consumptionPath(consumerKey)
	for {
		select {
		case <-ctx.Done():
			return renewal{}
		case <-done:
			return renewal{}
		case <-ticker.C:
			deadline := mongoTime(time.Now().Add(b.options.LeaseDuration))
			result, err := b.events.UpdateOne(ctx, bson.D{
				{Key: "_id", Value: claimed.event.ID},
				{Key: path + ".state", Value: stateRunning},
				{Key: path + ".lease.token", Value: claimed.token},
			}, bson.D{
				{Key: "$set", Value: bson.D{
					{Key: path + ".lease.deadline", Value: deadline},
				}},
			})
			if err != nil {
				if ctx.Err() != nil {
					return renewal{}
				}
				cancel()
				return renewal{err: fmt.Errorf("renew event delivery lease: %w", err)}
			}
			if result.MatchedCount == 0 {
				cancel()
				return renewal{lost: true}
			}
		}
	}
}

func (b *Bus) finish(
	ctx context.Context,
	consumerKey string,
	claimed *claimedEvent,
	handlerErr error,
	maxAttempts int,
	now time.Time,
) error {
	path := consumptionPath(consumerKey)
	filter := bson.D{
		{Key: "_id", Value: claimed.event.ID},
		{Key: path + ".state", Value: stateRunning},
		{Key: path + ".lease.token", Value: claimed.token},
	}
	set := bson.D{}
	if handlerErr == nil {
		set = bson.D{
			{Key: path + ".state", Value: stateAcked},
			{Key: path + ".lastError", Value: ""},
			{Key: path + ".completionTime", Value: now},
		}
	} else if eventbus.IsNoRetry(handlerErr) || claimed.attempt >= maxAttempts {
		set = bson.D{
			{Key: path + ".state", Value: stateDead},
			{Key: path + ".lastError", Value: handlerErr.Error()},
			{Key: path + ".completionTime", Value: now},
		}
	} else {
		delay, specified := eventbus.RetryDelay(handlerErr)
		if !specified {
			delay = defaultRetryBackoff(claimed.attempt)
		}
		set = bson.D{
			{Key: path + ".state", Value: statePending},
			{Key: path + ".lastError", Value: handlerErr.Error()},
			{Key: path + ".notBefore", Value: mongoTime(now.Add(delay))},
		}
	}
	if _, err := b.events.UpdateOne(ctx, filter, bson.D{
		{Key: "$set", Value: set},
		{Key: "$unset", Value: bson.D{
			{Key: path + ".lease", Value: ""},
		}},
	}); err != nil {
		return fmt.Errorf("finish event delivery: %w", err)
	}
	return nil
}

func (b *Bus) recoverLoop(ctx context.Context, consumerKey string) error {
	ticker := time.NewTicker(b.options.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if err := b.recoverExpired(ctx, consumerKey, mongoTime(time.Now())); err != nil {
			return err
		}
	}
}

func (b *Bus) recoverExpired(ctx context.Context, consumerKey string, now time.Time) error {
	path := consumptionPath(consumerKey)
	expired := bson.D{
		{Key: path + ".state", Value: stateRunning},
		{Key: path + ".lease.deadline", Value: bson.D{
			{Key: "$lte", Value: now},
		}},
	}
	dead := append(bson.D{}, expired...)
	dead = append(dead, bson.E{Key: "$expr", Value: bson.D{
		{Key: "$gte", Value: bson.A{
			"$" + path + ".attempt",
			"$" + path + ".lease.maxAttempts",
		}},
	}})
	if _, err := b.events.UpdateMany(ctx, dead, bson.D{
		{Key: "$set", Value: bson.D{
			{Key: path + ".state", Value: stateDead},
			{Key: path + ".lastError", Value: leaseExpiredError},
			{Key: path + ".completionTime", Value: now},
		}},
		{Key: "$unset", Value: bson.D{
			{Key: path + ".lease", Value: ""},
		}},
	}); err != nil {
		return fmt.Errorf("recover exhausted event deliveries: %w", err)
	}
	pending := append(bson.D{}, expired...)
	pending = append(pending, bson.E{Key: "$expr", Value: bson.D{
		{Key: "$lt", Value: bson.A{
			"$" + path + ".attempt",
			"$" + path + ".lease.maxAttempts",
		}},
	}})
	if _, err := b.events.UpdateMany(ctx, pending, bson.D{
		{Key: "$set", Value: bson.D{
			{Key: path + ".state", Value: statePending},
			{Key: path + ".lastError", Value: leaseExpiredError},
			{Key: path + ".notBefore", Value: now},
		}},
		{Key: "$unset", Value: bson.D{
			{Key: path + ".lease", Value: ""},
		}},
	}); err != nil {
		return fmt.Errorf("recover event deliveries: %w", err)
	}
	return nil
}

func insertWithClusterTime(document any, field string) mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{
			{Key: "$replaceWith", Value: bson.D{
				{Key: "$cond", Value: bson.D{
					{Key: "if", Value: bson.D{
						{Key: "$eq", Value: bson.A{
							bson.D{
								{Key: "$type", Value: "$" + field},
							},
							"missing",
						}},
					}},
					{Key: "then", Value: bson.D{
						{Key: "$mergeObjects", Value: bson.A{
							bson.D{
								{Key: "$literal", Value: document},
							},
							bson.D{
								{Key: field, Value: "$$CLUSTER_TIME"},
							},
						}},
					}},
					{Key: "else", Value: "$$ROOT"},
				}},
			}},
		},
	}
}

func consumptionPath(key string) string {
	return "consumptions." + key
}

func consumerKey(id string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
}

func (d eventDocument) event() eventbus.Event {
	return eventbus.Event{
		ID:       d.ID,
		Type:     d.Type,
		Source:   d.Source,
		Subject:  d.Subject,
		Time:     d.Time,
		Data:     bytes.Clone(d.Data),
		Metadata: maps.Clone(d.Metadata),
	}
}

func equivalentEvent(stored eventDocument, candidate eventbus.Event, idempotencyKey string) bool {
	return (candidate.ID == "" || candidate.ID == stored.ID) &&
		candidate.Type == stored.Type &&
		candidate.Source == stored.Source &&
		candidate.Subject == stored.Subject &&
		(candidate.Time.IsZero() || mongoTime(candidate.Time).Equal(stored.Time)) &&
		bytes.Equal(candidate.Data, stored.Data) &&
		maps.Equal(candidate.Metadata, stored.Metadata) &&
		(idempotencyKey == "" || idempotencyKey == stored.IdempotencyKey)
}

func cloneEvent(event eventbus.Event) eventbus.Event {
	event.Data = bytes.Clone(event.Data)
	event.Metadata = maps.Clone(event.Metadata)
	return event
}

func mongoTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Truncate(time.Millisecond)
}

func defaultRetryBackoff(attempt int) time.Duration {
	delay := 100 * time.Millisecond * time.Duration(1<<min(attempt-1, 8))
	return min(delay, 30*time.Second)
}

func eventTypeFilter(pattern string) bson.E {
	if !strings.ContainsAny(pattern, "*,{}") {
		return bson.E{Key: "type", Value: pattern}
	}
	parts := strings.Split(pattern, ".")
	for index, part := range parts {
		var remaining bool
		parts[index], remaining = eventTypeSectionRegexp(part)
		if remaining {
			if index == 0 {
				parts[index] = "[^.]+(?:\\.[^.]+)*"
			} else {
				parts[index-1] += "(?:\\.[^.]+)*"
				parts = parts[:index]
			}
			break
		}
	}
	return bson.E{
		Key:   "type",
		Value: primitive.Regex{Pattern: "^" + strings.Join(parts, "\\.") + "$"},
	}
}

func eventTypeSectionRegexp(section string) (string, bool) {
	if len(section) >= 2 && section[0] == '{' && section[len(section)-1] == '}' {
		section = section[1 : len(section)-1]
	}
	candidates := strings.Split(section, ",")
	for index, candidate := range candidates {
		if candidate == "**" {
			return "", true
		}
		if candidate == "*" {
			candidates[index] = "[^.]+"
		} else {
			candidates[index] = strings.ReplaceAll(regexp.QuoteMeta(candidate), `\*`, "[^.]*")
		}
	}
	if len(candidates) == 1 {
		return candidates[0], false
	}
	return "(?:" + strings.Join(candidates, "|") + ")", false
}
