# venn

venn, is play on `virtual ethereum node`, but like `venn-diagram` since it's like, the overlap between multiple rpcs/data sources. get it?

before the existence of erpc, there was a need to have a single rpc endpoint which could automatically fall back to other rpc endpoints. this tool was built internally at gfx labs to support this.

the key features that venn needed to be able to provide were:

1. cache blocks at head, to reduce the cost of multiple indexers.
2. fallback when rpcs fell behind or become unavailable, to a second rpc.
3. support eth_subscribe, as we have applications which depend on it.


the original version of venn - named brilliant - dates back to 2021.

# design notes / overview

## structure


It is an [fx](https://github.com/uber-go/fx) app (it originally wasn't, but was converted, as the firm switched to fx).

the configuration is `hcl`, like terraform, which can be found [here](./lib/config/config.go).

to understand the jsonrpc2 handling and semantics, please see [jrpc](https://gfx.cafe/open/jrpc) and the examples there. this is what powers the ability to interact with a single api, but serve http, websocket, or any other fd or abstract protocol (rabbitmq, nats) seamlessly

the primary entrypoint for the api handler is [here](./svc/handler/api.go). if you are familiar with chi, or go stdlib http handling, then this should be rather familiar to you.

there are two different types `stores`, a [headstore](./svc/stores/headstores) and [blockstore]('./svc/stores/vennstores').

the `headstore` provides a way to publish and read the near-head state of a chain, including headers, receipts, and any other information needed to honor subscriptions (eth_subscribe). this can be consumer by followers. the only existing implementation right now is redis.

the `blockstore` is a backend that can respond to json-rpc requests with historical headers and receipts, it could be a jsonrpc remote, a postgres database, or even another venn instance.

each `venn` cluster uses a leader election to determine who is the [stalker]('./svc/atoms/stalker/stalker.go'). the stalker is responsible for 'stalking' the head of the chain (being at the tip is the job for the node, we are by definition always stalking behind). the stalker reads data from redis, and so ideally there should be quick consistency here.

the stalker pushes new head payloads to the `headstore`, which is consumed by the [subcenter]('./svc/atoms/subcenter/component.go') to provide subscriptions. the stalker also reads from the headstore in order to serve requests at head. when indexing, this is by and large the #1 called.

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


#### hcl

i picked HCL for a few reasons.

1. i was doing a lot of devops work at this time, since the company was new, and I am the de-facto cloud engineer.
2. i wanted to be able to easily refer to other parts of a config (see the filters sections), since there are many classes of nodes.
3. yaml anchors are sort of demented, and jsonpointers felt somewhat clunky/unsafe.
4. i didn't want to go through the pain of writing an entire configuration api.

hcl gave me this weird middle ground between json/yaml, and a scripting language like starlark, javascript, or lua.

although, in hindsight, it would have perhaps been better to lean towards one side.

#### the app is multi-chain, instead of one chain per.

the oku backend is single-chain-tenanted, and it does make a lot of sense, from a devex point of view, to only ever be running a single chain at a time.

however, the bottleneck that this app addresses is the ability to parse json, understand the json, and then perform requests to external services. that means that for ideal scaling with minimal performance wastage, all chains should be combined, as you should scale this program with the absolute amount of rpc requests you need to serve, not the type of, or to which chain, the rpc requests are going to.

that said, i do still see the value of having a single-chain-per-app.
