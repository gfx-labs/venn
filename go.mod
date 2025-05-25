module gfx.cafe/gfx/venn

go 1.23.1

toolchain go1.23.2

require (
	gfx.cafe/open/gotoprom v0.0.2
	gfx.cafe/open/jrpc v0.7.0
	gfx.cafe/util/go v1.12.1-0.20240717000937-c42e930797c2
	github.com/alecthomas/kong v1.8.1
	github.com/alicebob/miniredis/v2 v2.32.1
	github.com/dranikpg/gtrs v0.6.0
	github.com/ethereum/go-ethereum v1.14.12
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-faster/jx v1.1.0
	github.com/go-redsync/redsync/v4 v4.12.1
	github.com/google/cel-go v0.20.1
	github.com/google/uuid v1.6.0
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/holiman/uint256 v1.3.2
	github.com/joho/godotenv v1.5.1
	github.com/lmittmann/tint v1.0.4
	github.com/nats-io/nats-server/v2 v2.11.3
	github.com/nats-io/nats.go v1.42.0
	github.com/prometheus/client_golang v1.19.1
	github.com/redis/go-redis/v9 v9.5.1
	github.com/redis/rueidis v1.0.59
	github.com/redis/rueidis/rueidislimiter v1.0.59
	github.com/riandyrn/otelchi v0.9.0
	github.com/stretchr/testify v1.10.0
	github.com/valyala/bytebufferpool v1.0.0
	github.com/wasilibs/go-re2 v1.4.1
	go.opentelemetry.io/otel v1.33.0
	go.opentelemetry.io/otel/trace v1.33.0
	go.uber.org/fx v1.22.1
	go.uber.org/multierr v1.11.0
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba
	golang.org/x/sync v0.13.0
	golang.org/x/sys v0.32.0
	golang.org/x/time v0.11.0
	sigs.k8s.io/yaml v1.4.0
)

require (
	gfx.cafe/open/websocket v1.10.0 // indirect
	github.com/alicebob/gopher-json v0.0.0-20230218143504-906a9b012302 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/c2h5oh/datasize v0.0.0-20231215233829-aa82cc1e6500 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gfx-labs/sse v0.0.0-20241016074208-406bd1302230 // indirect
	github.com/go-faster/errors v0.7.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/go-tpm v0.9.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.20.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mailgun/multibuf v0.2.0 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/jwt/v2 v2.7.4 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/redis/rueidis/rueidiscompat v1.0.59 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/tetratelabs/wazero v1.6.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.28.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.28.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.33.0 // indirect
	go.opentelemetry.io/otel/sdk v1.28.0 // indirect
	go.opentelemetry.io/proto/otlp v1.3.1 // indirect
	go.uber.org/dig v1.17.1 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/exp v0.0.0-20250106191152-7588d65b2ba8 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240701130421-f6361c86f094 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240701130421-f6361c86f094 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.35.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
