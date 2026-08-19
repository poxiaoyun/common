# cache

`cache` defines capability interfaces for expiring values and atomic expiring
counters. Higher-level modules compose these capabilities into database
caching, fixed-window limits, quotas, and similar behavior without coupling the
algorithm to a particular backend.

## Value cache

```go
type Cache[T any] interface {
    Get(ctx context.Context, key string) (value T, found bool, err error)
    Set(ctx context.Context, key string, value T, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
}
```

`Get` returns `found == false` and a nil error for a miss. `Set` replaces the
current value; a zero TTL means no expiration and a negative TTL returns
`ErrInvalidTTL`. `Delete` is idempotent.

A cache is best-effort storage and may evict an entry before its TTL. Database
or another authoritative source must therefore remain able to satisfy every
miss. Values passed across the interface are treated as immutable, including
maps, slices, pointers, and objects containing them.

Read-through loading and database invalidation belong to the module that owns
the authoritative source and its keys. They are not backend cache operations.

The process-local adapter requires an explicit capacity and evicts the least
recently used value when full:

```go
values, err := inmemory.New[User](1024)
```

Expiration is checked when a value is read. The adapter has no background
goroutine and requires no shutdown lifecycle.

## Windowed accumulation

```go
type WindowedAccumulator interface {
    Add(ctx context.Context, key string, amount int64) (Count, error)
}

type WindowedBudget interface {
    TryConsume(
        ctx context.Context,
        key string,
        amount int64,
    ) (Consumption, error)
}
```

`WindowedAccumulator` binds a positive window when its adapter is constructed.
`Add` atomically creates a missing counter at `amount` or adds `amount` to the
existing value. Creation fixes the expiration and later additions do not extend
it. It supports fixed-window attempt counts where additions may continue after
an upper-layer limit is exceeded.

`WindowedBudget` binds a positive capacity and window when its adapter is
constructed. `TryConsume` consumes a positive amount only when the resulting
usage does not exceed capacity. Rejection does not create or change state.
Requests, tokens, bytes, costs, and other weighted units can share this
capability without putting those domains into the cache interface.

Unlike ordinary cached values, counter state must not be evicted before its
expiration. A backend configured for early eviction cannot implement either
windowed capability. Operations are atomic for one key; different keys and
different capabilities do not share a transaction. A remote error may leave an
operation's commit status unknown, so the interfaces do not promise
exactly-once retries.

The process-local adapters are constructed with immutable policy:

```go
attempts, err := inmemory.NewWindowedAccumulator(time.Minute)
tokens, err := inmemory.NewWindowedBudget(200_000, time.Minute)
```

Each instance manages many subject keys under one policy. It retains every live
key and removes expired state during subsequent operations. It has no
background goroutine or shutdown lifecycle; memory usage is proportional to
the number of live keys.

Adapters bind namespace, window, capacity when applicable, serialization, and
backend configuration when constructed. A changed policy should use a newly
configured instance and an isolated namespace. Callers accept only the
capability interface they actually need.

## Fixed-window limiting

`NewFixedWindowLimiter` binds a positive limit to a configured
`WindowedAccumulator`. `Allow` consumes one attempt for a key and returns
whether it is allowed, how many attempts remain, and the authoritative reset
time:

```go
counter, err := inmemory.NewWindowedAccumulator(time.Minute)
if err != nil {
    return err
}

limiter, err := cache.NewFixedWindowLimiter(counter, 100)
if err != nil {
    return err
}

decision, err := limiter.Allow(ctx, accountID)
if err != nil {
    return err
}
if !decision.Allowed {
    return ErrRateLimited
}
```

The window begins with the first attempt for a missing key rather than at a
wall-clock boundary. Rejected attempts are counted and do not extend the
window.

Counter failures are returned to the caller. The application owns the decision
to fail open or fail closed because that choice depends on what is being
protected.
