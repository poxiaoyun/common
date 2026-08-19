# Cache design

The `cache` module defines small capability interfaces implemented by cache-like
backends. Higher-level modules depend on the narrowest capability required by
their algorithm. A backend may implement several capabilities, but an adapter
must not advertise a capability whose guarantees it cannot provide.

## Seams

`Cache[T]` is the seam for best-effort expiring values. It owns the observable
semantics of a cache hit, replacement, expiration, and deletion. Key
construction, read-through loading, database invalidation, negative caching,
and failure policy remain with higher-level modules.

`WindowedAccumulator` is the seam for atomically accumulating positive weighted
amounts. Its window is bound when an adapter is constructed. A missing or
expired key starts a window; later additions cannot change its expiration. The
state cannot be silently evicted before expiration.

`WindowedBudget` is the seam for atomically consuming positive weighted amounts
without exceeding a capacity. Capacity and window are bound when an adapter is
constructed because both participate in the atomic state transition. A
rejected consumption does not create or mutate state. Requests, tokens, bytes,
costs, and similar units can share this capability.

`Limiter` is the caller-facing seam for consuming one attempt. The built-in
`FixedWindowLimiter` binds one positive limit and accepts an already configured
`WindowedAccumulator`. Its window begins when a missing counter is first added;
it is not aligned to wall-clock boundaries. Rejected attempts are still counted
and never extend the current window.

Namespaces and serialization belong to adapter construction. They do not
appear on recurring operations because callers should not have to repeat or
coordinate them for every access.

The in-memory value-cache adapter requires an explicit positive capacity and
uses least-recently-used eviction. Expiration is checked lazily on access.
Expired entries may occupy capacity until accessed or evicted, but memory stays
bounded. The adapter owns no background goroutine and has no close lifecycle.

The in-memory accumulator and budget adapters bind their policy in constructors
and have no capacity eviction because active state must remain present until
expiration. Each keeps expirations in a min-heap and removes expired state at
the start of an operation, so memory is proportional to live keys. They own no
background lifecycle.

## Invariants

- A cache miss is represented only by `found == false` with a nil error. Zero
  values can be cached normally.
- Cache operations are individually atomic for one key. A sequence of calls is
  not a transaction.
- Cache entries may be evicted before their TTL. Callers must remain correct on
  every miss.
- Values are immutable across the seam. Callers must not mutate a value after
  passing it to `Set` or mutate a value returned by `Get`.
- A zero cache TTL means no expiration. A negative cache TTL is invalid.
- Accumulator and budget amounts must be positive signed 64-bit integers. An
  invalid amount or accumulator overflow does not mutate state.
- `Add` atomically creates or advances one counter by the requested amount.
  Creation fixes its expiration; later additions do not extend it.
- `TryConsume` atomically consumes the requested amount only when the result
  does not exceed the capacity bound to the adapter. Rejection leaves existing
  and missing state unchanged.
- Windows are positive and remain present until expiration. At expiration, the
  next successful operation creates a new window.
- The adapter's clock determines expiration. `Count.ExpiresAt` reports that
  authoritative deadline.
- Each configured adapter owns one policy scope and manages many subject keys.
  Namespace and policy versioning keep incompatible scopes isolated.
- Counter failures are returned to the limiter caller. Whether to allow or deny
  after an infrastructure failure remains the caller's policy.
- Operations are atomic for one key only. Different keys and different
  capabilities do not share a transaction.
- A remote error may leave an operation's commit status unknown. The interfaces
  do not promise exactly-once retries.

Batch access, conditional writes, comparison-and-swap, read-through loading,
and cache clearing are not part of the current interface. They should be added
only when a higher-level module requires their exact semantics.
