package prom

import (
	"gfx.cafe/open/gotoprom"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	for _, v := range []any{
		&Requests,
		&Remotes,
		&Stalker,
		&Gateway,
		&RemoteHealth,
		&ChainHealth,
	} {
		gotoprom.MustInit(v, "venn", nil)
	}
}

type GatewayRequestLabel struct {
	Endpoint string `label:"endpoint"`
	Target   string `label:"target"`
	Method   string `label:"method"`
	Success  bool   `label:"success"`
}

var Gateway struct {
	RequestLatency      func(label GatewayRequestLabel) prometheus.Histogram `name:"gateway_request_latency_ms" help:"The total latency of each request in milliseconds" buckets:"1,10,50,100,250,500,1000,2000,5000,10000,50000"`
	SubscriptionCreated func(label GatewayRequestLabel) prometheus.Counter   `name:"gateway_subscription_created" help:"The total number of subscriptions opened"`
	SubscriptionClosed  func(label GatewayRequestLabel) prometheus.Counter   `name:"gateway_subscription_closed" help:"The total number of subscriptions closed"`
}

type RequestLabel struct {
	Chain   string `label:"chain"`
	Method  string `label:"method"`
	Success bool   `label:"success"`
}

var Requests struct {
	Latency func(label RequestLabel) prometheus.Histogram `name:"request_latency_ms" help:"The total latency of each request in milliseconds" buckets:"1,10,50,100,250,500,1000,2000,5000,10000,50000"`
}

type RemoteLabel struct {
	Chain   string `label:"chain"`
	Remote  string `label:"remote"`
	Method  string `label:"method"`
	Success bool   `label:"success"`
}

var Remotes struct {
	Latency func(label RemoteLabel) prometheus.Histogram `name:"remote_latency_ms" help:"The total latency of each request in milliseconds" buckets:"1,10,50,100,250,500,1000,2000,5000,10000,50000"`
}

type StalkerLabel struct {
	Chain string `label:"chain"`
}

var Stalker struct {
	PropagationDelayMean  func(label StalkerLabel) prometheus.Gauge     `name:"propagation_delay_ms" help:"the mean propogation delay for the chain"`
	BlockPropagationDelay func(label StalkerLabel) prometheus.Histogram `name:"block_propagation_delay_ms" help:"the delay of propogation for the blocks" buckets:"1,10,50,100,250,500,1000,2000,3000,4000,5000,6000,8000,9000,10000,12000,24000,30000"`
	HeadBlock             func(label StalkerLabel) prometheus.Gauge     `name:"stalker_head_block" help:"the head block for the chain"`
}

type RemoteHealthLabel struct {
	Chain  string `label:"chain"`
	Remote string `label:"remote"`
}

var RemoteHealth struct {
	Status               func(label RemoteHealthLabel) prometheus.Gauge     `name:"remote_health_status" help:"Health status of remote endpoint: 1=healthy, 0=unhealthy, -1=unknown"`
	CheckLatency         func(label RemoteHealthLabel) prometheus.Histogram `name:"remote_health_check_latency_ms" help:"Latency of health checks in milliseconds" buckets:"1,10,50,100,250,500,1000,2000,5000,10000,30000"`
	CheckFailures        func(label RemoteHealthLabel) prometheus.Counter   `name:"remote_health_check_failures_total" help:"Total number of health check failures"`
	LastSuccessTimestamp func(label RemoteHealthLabel) prometheus.Gauge     `name:"remote_health_last_success_timestamp" help:"Timestamp of last successful health check"`
}

type ChainHealthLabel struct {
	Chain string `label:"chain"`
}

var ChainHealth struct {
	AvailabilityPercent func(label ChainHealthLabel) prometheus.Gauge `name:"chain_availability_percent" help:"Percentage of successful requests to the chain"`
	HealthyRemoteCount  func(label ChainHealthLabel) prometheus.Gauge `name:"chain_healthy_remote_count" help:"Number of healthy remotes for the chain"`
	TotalRemoteCount    func(label ChainHealthLabel) prometheus.Gauge `name:"chain_total_remote_count" help:"Total number of configured remotes for the chain"`
	RequestSuccessRate  func(label ChainHealthLabel) prometheus.Gauge `name:"chain_request_success_rate" help:"Success rate of requests to the chain (rolling average)"`
}
