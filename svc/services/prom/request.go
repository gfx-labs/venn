package prom

import (
	"github.com/prometheus/client_golang/prometheus"
)

type RequestLabel struct {
	Chain   string `label:"chain"`
	Method  string `label:"method"`
	Success bool   `label:"success"`
}

type Requests struct {
	Latency func(label RequestLabel) prometheus.Histogram `name:"latency_ms" help:"The total latency of each request in milliseconds" buckets:"1,10,50,100,250,500,1000,2000,5000,10000,50000"`
}
