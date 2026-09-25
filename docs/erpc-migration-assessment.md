# venn → erpc migration assessment

Investigation of whether erpc can replace venn for the indexing use case, and how
invasive the missing pieces are. Based on erpc `main` as of 2026-09-25.

## Summary

| Requirement | erpc today | Lift |
| --- | --- | --- |
| Only one remote hit when populating caches | Partial. In-process request coalescing exists; hedge is ON by default and fans out; no cross-replica coalescing | **Small (config) + Medium (cross-replica)** |
| Tiered load balancing: try all of tier A before any of tier B | **Yes, already supported** via `selectionPolicy` + tags + `preferTag` | **None (config only)** |
| Artificial websocket subscriptions (`eth_subscribe`) | **Absent.** No inbound WS, no outbound WS client, no subscription synthesis | **Large** |
| Shared-state cache to save cost | Partial. Cache connectors are shared (redis/pg), but shared-state registry is counters only; no shared head store | **Small (config) + Medium (head store)** |

Verdict: two of four are config-only. The genuinely invasive one is websocket
subscriptions. A fork is reasonable, but scope it narrowly: the subscription
work is additive (new transport + new component) rather than a rewrite of
erpc internals, so rebasing on upstream should stay tractable.

## 1. Only hitting one remote when populating caches

### What exists

In-process in-flight coalescing ("multiplexing") is real and on by default:

- `erpc/networks.go:1860-1882` — `handleMultiplexing` runs **before** the cache lookup.
- `erpc/networks.go:3107-3144` — `LoadOrStore` on the network's `inFlightRequests` map keyed by `req.CacheHash()`; first caller is leader, rest are followers.
- `erpc/networks.go:3175-3203` — followers wait and copy the leader's response.
- `erpc/multiplexer.go:36-87` — response is cloned per follower.
- `common/config.go:2257,2290-2295` — `network.multiplexing`, nil means enabled.

So N identical concurrent requests to one erpc pod produce one upstream call.

### What breaks the "one remote" property

1. **Hedge is enabled by default** and issues a parallel duplicate request to a
   second upstream: `common/defaults.go:137-141` sets `Delay: {Quantile: 0.7}`,
   `MaxCount: 2` on the network-level `*` failsafe. That is up to 3 concurrent
   upstream calls for one logical request. Disable with `hedge.maxCount: 0`
   (`docs/pages/config/failsafe/hedge.mdx:173,399`).
2. **Consensus policy** fans out to `maxParticipants` upstreams
   (`common/config.go:1805-1815`). Off unless configured. Leave it off.
3. **Retry** defaults to `maxAttempts: 5` at network level
   (`common/defaults.go:127-133`), each attempt sweeping to the next upstream
   (`erpc/network_executor.go:151-163,227-285`; next pick at
   `erpc/networks.go:2211` `NextUpstream()`). This is sequential, not fanout, so
   it costs money only on failure. Acceptable, but lower it.
4. **Integrity autocorrection** can hunt for a replacement upstream via
   retry/hedge: `common/config_integrity.go:25-38`. Note
   `EnforceHighestBlock: true` is a default (`common/defaults.go:116-119`).
5. **Cache connector fanout** is parallel across matched connectors
   (`architecture/evm/json_rpc_cache.go:227-271`) but that hits caches, not
   paid remotes, so it is fine.

### The real gap: no cross-replica coalescing

Multiplexing is a per-process in-memory map. Ten indexer-facing erpc pods
cache-missing the same `eth_getLogs` produce ten upstream calls. erpc's shared
state cannot help as-is: `SharedStateRegistry` is explicitly `CounterInt64`-only
(`data/shared_state_registry.go:38-55`), and redis pub/sub pattern-subscribes
only to `counter:*` (`data/redis_pubsub_manager.go:154-168`). No request bodies
or responses cross replicas.

venn solves the head case differently: leader election means exactly one cluster
member polls the head (`svc/node/atoms/stalker/stalker.go:52-91`,
`lib/election/strategy.go:24-35`, `lib/election/redsync.go:40-73`), and everyone
else reads the shared headstore. venn also has a per-process singleflight on the
blockstore (`lib/stores/blockstore/deduper.go:24-47`), same scope as erpc's.

**Lift:** config-only for the fanout sources (small). A distributed lock around
cache population is a medium, additive change: wrap the cache-miss path with a
redis lock keyed by `CacheHash()`, waiters poll the shared cache connector. The
hook point is clean — between `handleMultiplexing` and the upstream sweep at
`erpc/networks.go:1880-1894`. It does not require touching upstream selection or
the failsafe engine.

## 2. Tiered load balancing (cheap first, then fallback)

**Already supported. No code change needed.**

erpc's `selectionPolicy` is a JS function `(upstreams, ctx) => Upstream[]`
evaluated on a timer, whose result is the per-request upstream order:

- `common/config.go:2741-2776` — `evalFunc`, `evalInterval`, `evalScope`.
- `internal/policy/engine.go:372-390` — requests read the cached slot order.
- `erpc/networks.go:1900-1920` — `GetOrdered`/`GetOrderedInLane` supplies the order.

The stdlib's `preferTag` is exactly the tier primitive you want
(`internal/policy/stdlib/stdlib.js:931-942`): it selects the subset matching a
tag glob provided at least `minHealthy` match, else falls through to the
`fallback` pattern. The bundled default policy already ships a two-tier split
(`internal/policy/default_policy.js`):

```js
.preferTag('!tier:fallback', { minHealthy: 1, fallback: 'tier:fallback' })
```

Tag cheap upstreams `tier:cheap` and expensive ones `tier:fallback`, then chain
`preferTag` calls for more tiers. The health excludes above it
(`errorRateAbove`, `blockNumberLagAbove(16)`, `blockSecondsLagAbove(30)`,
`latencyAbove`) give you venn's "fell behind or unavailable" fallback for free,
matching venn's `Doctor`/`Backer` (`lib/callcenter/doctor.go:28-80,96-200`,
`lib/callcenter/backer.go:15-53,56-123`).

erpc is strictly more expressive here than venn, whose tiers are integer
priorities assigned from remote list order with round-robin inside a tier
(`lib/callcenter/cluster.go:16-67,76-119`).

**One caveat:** policy eval is tick-based, not re-run per request failure
(`internal/policy/slot.go:123-145,171`). A tier-A upstream that just died is
still in the cached order until the next tick; the retry sweep handles that
within a request, so behavior is correct, it just costs one failed attempt.
Set `evalInterval` low if that matters.

## 3. Artificial websocket subscriptions

**Fully absent. This is the invasive piece.**

- No inbound WS upgrade. `erpc/http_server.go:39-56,151-160,246` is plain
  `net/http` request/response. A test comment states it outright:
  `erpc/networks_availability_upper_race_test.go:709-712`.
- No outbound WS client. `clients/registry.go:100-101` returns
  `websocket client not implemented yet` for `ws`/`wss`. SVM is HTTP-only
  (`clients/registry.go:122-142`).
- No subscription synthesis anywhere. `architecture/evm/evm_state_poller.go:49-67`
  tracks upstream head state but has no publish/subscribe API and emits no
  JSON-RPC notifications.

venn's implementation is the template and it is not large. `Subcenter`
(`svc/node/atoms/subcenter/component.go:85-237`) intercepts `eth_subscribe`,
requires a notifier-capable connection, and:

- `newHeads` (`:132-170`): watch headstore, fetch each intervening block with
  `eth_getBlockByNumber(n, false)`, strip `transactions`, notify.
- `logs` (`:171-233`): on head update, `eth_getLogs` from `current+1` to new
  head with the subscriber's addresses/topics, emit each log.
- `newPendingTransactions`: not implemented locally; other kinds error (`:234-237`).
- Real upstream WS passthrough exists separately (`lib/callcenter/proxier.go:29-79`).

### What a fork needs to add

Additive, roughly in dependency order:

1. **Inbound WS transport** in `erpc/http_server.go`: upgrade detection,
   read/write pump, per-connection subscription registry, cancel on disconnect.
   Decide how project auth, rate limits, and method allow/ignore apply to a
   long-lived session (erpc applies these per request today at
   `erpc/http_server.go:452-483,519`).
2. **Method interception** for `eth_subscribe`/`eth_unsubscribe` before the
   normal forward path (`erpc/networks.go:1800` `Network.Forward`,
   `erpc/projects.go:130`).
3. **Subscription manager**: keyed by (connection, generated sub id), holds
   filter + last-delivered cursor, serializes `eth_subscription` notifications,
   fans out without blocking the poller, idempotent unsubscribe.
4. **Head event source**: give `evm_state_poller.go` a publish API. It already
   polls and already writes latest/finalized into shared state
   (`architecture/evm/evm_state_poller.go:154-155`), so this is the smallest
   step. Define reorg, skipped-height, and duplicate-head behavior.
5. **`logs` polling** through the existing `eth_getLogs` path
   (`architecture/evm/eth_getLogs.go`), with dedup and removed-log semantics.

Estimate: this is the bulk of the fork. It touches the HTTP server and adds a
new component, but it does not modify the cache, failsafe, or selection
subsystems, which is what keeps rebases survivable.

## 4. Shared-state caching

### What exists

The cache is genuinely shared when backed by redis/postgres/dynamo, and it is
finality-aware:

- Stores result bytes with optional zstd, never caches errors:
  `architecture/evm/json_rpc_cache.go:813-849,1158-1224`.
- Keying: `partitionKey = <networkId>:<blockRef>`, `rangeKey = <CacheHash>`
  (`:1235-1249`); reverse index for wildcard reads (`:1084-1120`).
- Policies match network/method/params/finality, can be get-only or set-only:
  `data/cache_policy.go:82-169`.
- TTL fixed or derived from block time: `data/cache_policy.go:279-295`.
- Redis connector passes TTL through and maintains reverse lookups:
  `data/redis.go:334-370`.

Crucially, near-head data **is** cacheable: `unfinalized` and `realtime` are
valid policy finalities (`data/cache_policy.go:119-169`). So the venn "cache
blocks at head to stop N indexers hammering the same RPC" goal is expressible
today as a shared-redis policy with `finality: realtime` and a short TTL.

### Gaps vs venn

1. **No shared head store.** erpc has no distributed store of headers/receipts/
   logs at the tip. The shared-state registry is counters only
   (`data/shared_state_registry.go:38-55`; consumers at
   `architecture/evm/evm_state_poller.go:154-155,886`,
   `upstream/upstream.go:1556`). There is a `CacheHeadReporter` capability
   (`data/connector.go:53-60`) but it reports freshness metadata, not payloads.
   There is even a TODO about subscription-driven pre-population
   (`architecture/evm/json_rpc_cache.go:646-647`).
   In practice a shared redis cache policy at `realtime` finality covers most of
   this. A true head store is only needed to back `newHeads` cheaply, so fold it
   into the subscription work.
2. **No reorg invalidation of cache entries.** Near-tip values age out by TTL
   and a read-side age guard; under `enforceHighestBlock` a realtime response
   behind the known tip is not written (`json_rpc_cache.go:1179-1203`). Shared
   counters do handle rollback with callbacks
   (`data/shared_state_variable.go:234-285,300-389`) but that is head tracking,
   not a cache purge. venn's LRU purges its number index on parent-hash mismatch
   (`lib/stores/blockstore/lru.go:80-109`). With short realtime TTLs this is
   mostly moot; if you cache unfinalized aggressively you need an invalidation
   hook.

## Recommended plan

**Phase 0, config only, no fork.** Prove out erpc as-is:

- `hedge.maxCount: 0` on the network `*` failsafe to stop duplicate fanout.
- No `consensus` policy.
- Lower network `retry.maxAttempts` from the default 5.
- Tag upstreams `tier:cheap` / `tier:fallback`; use `preferTag` in
  `selectionPolicy.evalFunc`. Short `evalInterval`.
- Shared redis cache connector with policies per finality, including a
  `realtime` policy with a short TTL for head data.
- Keep `multiplexing` enabled.

This gets you tiering, fallback-on-lag, and shared caching with zero code.
Measure remote call volume here before writing anything.

**Phase 1, fork, small.** Distributed cache-population lock keyed by
`CacheHash()` at `erpc/networks.go:1880-1894`. Removes cross-replica duplicate
upstream calls. Self-contained, low rebase risk.

**Phase 2, fork, large.** Websocket transport plus synthetic `newHeads` and
`logs` subscriptions, per the sketch in section 3. Add a head publish API to
`evm_state_poller.go` rather than a separate stalker, since erpc's poller
already does the polling and already shares head counters.

**Upstreaming.** Phase 1 and the poller publish API in phase 2 are plausibly
acceptable upstream; the erpc codebase already has a TODO pointing at
subscription-driven cache population. Proposing them upstream before forking
would shrink the fork to the transport layer.
