# venn

venn, is play on `virtual ethereum node`, but like `venn-diagram` since it's like, the overlap between multiple rpcs/data sources. get it?

before the existence of erpc, there was a need to have a single rpc endpoint which could automatically fall back to other rpc endpoints. this tool was built internally at gfx labs to support this. importantly, we wanted to send requests primarily to lower quality rpc endpoints, but be able to fallback to high quality ones, as a cost optimization.

the key features that venn needed to be able to provide (and does) are:

1. cache blocks at head, to reduce the cost of multiple indexers both running to the same rpc.
2. fallback when rpcs fell behind or become unavailable, to a second rpc.
3. support eth_subscribe, as we have applications which depend on it.


there are an assortment of other features - but by in large the are not super relevant.

the original version of venn - named brilliant - dates back to 2021.

# design notes / overview

## structure


It is an [fx](https://github.com/uber-go/fx) app (it originally wasn't, but was converted, as the firm switched to fx).

the configuration is `hcl`, like terraform, which can be found [here](./lib/config/config.go).

to understand the jsonrpc2 handling and semantics, please see [jrpc](https://gfx.cafe/open/jrpc) and the examples there. this is what powers the ability to interact with a single api, but serve http, websocket, or any other fd or abstract protocol (rabbitmq, nats) seamlessly

the primary entrypoint for the api handler is [here](./svc/handler/api.go). if you are familiar with chi, or go stdlib http handling, then this should be rather familiar to you.

there are two different types `stores`, a [headstore](./svc/stores/headstores) and [blockstore](./svc/stores/vennstores).

the `headstore` provides a way to publish and read the near-head state of a chain, including headers, receipts, and any other information needed to honor subscriptions (eth_subscribe). this can be consumer by followers. the only existing implementation right now is redis.

the `blockstore` is a backend that can respond to json-rpc requests with historical headers and receipts, it could be a jsonrpc remote, a postgres database, or even another venn instance.

each `venn` cluster uses a leader election to determine who is the [stalker](./svc/atoms/stalker/stalker.go). the stalker is responsible for 'stalking' the head of the chain (being at the tip is the job for the node, we are by definition always stalking behind). the stalker reads data from redis, and so ideally there should be quick consistency here.

the stalker pushes new head payloads to the `headstore`, which is consumed by the [subcenter](./svc/atoms/subcenter/component.go) to provide subscriptions. the stalker also reads from the headstore in order to serve requests at head. when indexing, this is by and large the #1 called.

a [forger](./svc/atoms/forger) allows the forging of json-rpc methods that the original remotes do not support

now, you can understand the routing. each request will

1. the subcenter, if it's a subscription
2. the forger (if its a method that needs to be forged)
3. the stalker (head requests, the 'head' rpc)
4. the caches (this is an intelligent cache, which uses different blockstores)
5. the actual load balancer, which is a set of remotes.

each request can also ask itself for data, for instance, the forger uses the stalker, caches, and remotes, but that access is handled abstractly, so you get the advantages of the stalker + caching, without worrying about it


for instance:

a typical eth_call request will go to the actual load balancer, as the previous steps all do not match the method.

an eth_blockNumber request will go to the stalker.

an eth_getBlockByNumber request will go to the stalker, if it's at head, and otherwise the cache if not the load balancers

an eth_getLogs request for historical information will go to the caches, then the actual load balancer if there is a cache miss


### some weird things about this repo / project

#### weird layout/structures

as previously mentioned, we converted from not-fx to fx, so there are some rough edges around the structure. this is a very old project within gfx.


#### the app is multi-chain, instead of one chain per.

the oku backend is single-chain-tenanted, and it does make a lot of sense, from a devex point of view, to only ever be running a single chain at a time.

however, the bottleneck that this app addresses is the ability to parse json, understand the json, and then perform requests to external services. that means that for ideal scaling with minimal performance wastage, all chains should be combined, as you should scale this program with the absolute amount of rpc requests you need to serve, not the type of, or to which chain, the rpc requests are going to.

that said, i do still see the value of having a single-chain-per-app.

# development

## building and testing

the project uses go 1.23+ and can be built with:

```bash
go build ./cmd/venn
```

## continuous integration

the project includes automated ci for pull requests that:

- validates go formatting with `gofmt`
- runs `go vet` for static analysis  
- builds the application
- runs all unit tests including race detection
- optionally runs integration tests (may fail if external services are unavailable)

ci runs automatically on pull requests targeting main, master, or develop branches. ensure your code passes `gofmt -w .` before submitting prs.

### Gateway mapping (optional)

See `gateway.solana.yml.sample`:

```yaml
endpoint:
  venn_url: http://localhost:8545
  paths:
    sol: solana
  methods:
    - getLatestBlockhash
    - getBlockHeight
    - getSlot
    - getGenesisHash
    - getVersion
    - getHealth
```

## Non‑EVM Blockchain

### Solana

Venn supports Solana over HTTP (no websockets). Enable per chain with `protocol: solana`.

Example (see `venn.yml.sample` for a complete sample):

```yaml
chains:
  - name: solana
    id: 101
    protocol: solana
    block_time_seconds: 0.4
    solana:
      network: mainnet-beta
      genesis_hash: 5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp
      head_method: getBlockHeight   # or getSlot
    remotes:
      - name: solana-main
        url: https://api.mainnet-beta.solana.com
        priority: 100
      - name: backup
        url: https://solana-mainnet.rpcpool.com
        priority: 90
```

Health: uses `getBlockHeight`/`getSlot` and optionally `getGenesisHash` (tolerant to provider suffixes). `getHealth` is counted as a bonus signal.

Stalker: updates head via `head_method`, sleeping roughly `block_time_seconds` between polls. Non‑EVM chains are not request‑gated by health.

Quick test:s

```bash
curl http://localhost:8545/solana \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "getLatestBlockhash"
  }'
```

## NEAR

Enable per chain with `protocol: near`.

Example (see `venn.yml.sample`):

```yaml
chains:
  - name: near
    id: 0
    protocol: near
    block_time_seconds: 1.2
    near:
      network_id: mainnet
      finality: final
      genesis_hash: ""
    remotes:
      - name: near-main
        url: https://rpc.mainnet.near.org
        priority: 100
```

Quick curl:

```bash
curl http://localhost:8545/near \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "block",
    "params": {"finality":"final"}
  }'
```
