# Venn Prometheus Metrics

This document provides a comprehensive reference for all Prometheus metrics exposed by Venn. These metrics enable monitoring of remote endpoint health, request performance, chain availability, and network propagation characteristics.

## Overview

Venn exposes metrics across several key areas:
- **Gateway Metrics** - HTTP/WebSocket gateway performance and subscriptions
- **Request Metrics** - JSON-RPC request latency and success rates
- **Remote Endpoint Metrics** - Performance and health of individual remote providers
- **Remote Health Metrics** - Detailed health monitoring of remote endpoints
- **Chain Health Metrics** - Aggregated chain-level health and availability
- **Stalker Metrics** - Block propagation and network timing data

---

## Gateway Metrics

These metrics track performance and usage of the HTTP/WebSocket gateway that serves client requests.

### `venn_gateway_request_latency_ms`
**Type:** Histogram  
**Labels:** `endpoint`, `target`, `method`, `success`  
**Buckets:** 1, 10, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 50000  
**Description:** Total latency of gateway requests in milliseconds, including routing overhead

**Example:**
```
venn_gateway_request_latency_ms{endpoint="api",target="ethereum",method="eth_getBalance",success="true"} 120
```

### `venn_gateway_subscription_created`
**Type:** Counter  
**Labels:** `endpoint`, `target`, `method`, `success`  
**Description:** Total number of WebSocket subscriptions opened

### `venn_gateway_subscription_closed`
**Type:** Counter  
**Labels:** `endpoint`, `target`, `method`, `success`  
**Description:** Total number of WebSocket subscriptions closed

---

## Request Metrics

These metrics track JSON-RPC requests processed by Venn, regardless of which remote endpoint handled them.

### `venn_request_latency_ms`
**Type:** Histogram  
**Labels:** `chain`, `method`, `success`  
**Buckets:** 1, 10, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 50000  
**Description:** End-to-end latency of JSON-RPC requests in milliseconds

**Example:**
```
venn_request_latency_ms{chain="ethereum",method="eth_blockNumber",success="true"} 45
venn_request_latency_ms{chain="ethereum",method="eth_getBalance",success="false"} 2500
```

**Use Cases:**
- Monitor overall request performance across all remote endpoints
- Track success rates for specific RPC methods
- Identify performance degradation patterns

---

## Remote Endpoint Metrics

These metrics track performance of individual remote endpoints (Alchemy, Infura, etc.).

### `venn_remote_latency_ms`
**Type:** Histogram  
**Labels:** `chain`, `remote`, `method`, `success`  
**Buckets:** 1, 10, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 50000  
**Description:** Latency of requests to specific remote endpoints in milliseconds

**Example:**
```
venn_remote_latency_ms{chain="ethereum",remote="alchemy",method="eth_getBalance",success="true"} 89
venn_remote_latency_ms{chain="ethereum",remote="infura",method="eth_getBalance",success="true"} 156
```

**Use Cases:**
- Compare performance between different remote providers
- Identify which remotes are fastest for specific methods
- Track remote-specific error rates

---

## Remote Health Metrics

These metrics provide detailed health monitoring for each remote endpoint, updated during periodic health checks.

### `venn_remote_health_status`
**Type:** Gauge  
**Labels:** `chain`, `remote`  
**Description:** Current health status of remote endpoint
- `1` = Healthy (passing health checks)
- `0` = Unhealthy (failing health checks)  
- `-1` = Unknown (status not yet determined)

**Example:**
```
venn_remote_health_status{chain="ethereum",remote="alchemy"} 1
venn_remote_health_status{chain="ethereum",remote="infura"} 0
```

### `venn_remote_health_check_latency_ms`
**Type:** Histogram  
**Labels:** `chain`, `remote`  
**Buckets:** 1, 10, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 30000  
**Description:** Latency of health check requests in milliseconds

Health checks verify both `eth_blockNumber` and `eth_chainId` to ensure remotes are responsive and on the correct chain.

### `venn_remote_health_check_failures_total`
**Type:** Counter  
**Labels:** `chain`, `remote`  
**Description:** Total number of health check failures since startup

### `venn_remote_health_last_success_timestamp`
**Type:** Gauge  
**Labels:** `chain`, `remote`  
**Description:** Unix timestamp of the last successful health check

**Use Cases:**
- Alert when remotes become unhealthy: `venn_remote_health_status == 0`
- Track health check performance over time
- Monitor how long remotes have been unhealthy

---

## Chain Health Metrics

These metrics provide aggregated health overviews at the chain level, updated every 30 seconds.

### `venn_chain_availability_percent`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Percentage of healthy remotes for the chain (0-100)

Calculated as: `(healthy_remotes / total_remotes) * 100`

**Example:**
```
venn_chain_availability_percent{chain="ethereum"} 75.0
venn_chain_availability_percent{chain="polygon"} 100.0
```

### `venn_chain_healthy_remote_count`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Number of currently healthy remote endpoints

### `venn_chain_total_remote_count`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Total number of configured remote endpoints

### `venn_chain_request_success_rate`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Rolling average success rate of requests to the chain (0-100)

Based on actual request outcomes aggregated across all remotes using a 1-minute rolling window.

**Use Cases:**
- Alert on zero healthy remotes: `venn_chain_healthy_remote_count == 0`
- Monitor chain availability trends: `venn_chain_availability_percent < 50`
- Track request success rates: `venn_chain_request_success_rate < 95`

---

## Stalker Metrics

These metrics track block propagation and network timing characteristics for chains with stalking enabled.

### `venn_propagation_delay_ms`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Mean block propagation delay for the chain in milliseconds

Tracks how long it takes for new blocks to be observed by Venn after they should have been produced according to the expected block time.

### `venn_block_propagation_delay_ms`
**Type:** Histogram  
**Labels:** `chain`  
**Buckets:** 1, 10, 50, 100, 250, 500, 1000, 2000, 3000, 4000, 5000, 6000, 8000, 9000, 10000, 12000, 24000, 30000  
**Description:** Individual block propagation delays in milliseconds

### `venn_stalker_head_block`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Current head block number observed by the stalker

**Example:**
```
venn_stalker_head_block{chain="ethereum"} 18500000
venn_propagation_delay_ms{chain="ethereum"} 150
```

**Use Cases:**
- Monitor network health and block propagation speed
- Detect if Venn is falling behind the chain head
- Track synchronization performance

---

## Alerting Rules

### Critical Alerts

```yaml
# No healthy remotes available for a chain
- alert: ChainNoHealthyRemotes
  expr: venn_chain_healthy_remote_count == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "No healthy remotes available for chain {{ $labels.chain }}"
    description: "Chain {{ $labels.chain }} has no healthy remote endpoints, requests will fail"

# Chain availability critically low
- alert: ChainAvailabilityCritical
  expr: venn_chain_availability_percent < 25
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Chain {{ $labels.chain }} availability critically low: {{ $value }}%"
    description: "Less than 25% of remotes are healthy for chain {{ $labels.chain }}"

# Gateway request latency too high
- alert: GatewayLatencyHigh
  expr: histogram_quantile(0.95, rate(venn_gateway_request_latency_ms_bucket[5m])) > 2000
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Gateway 95th percentile latency is {{ $value }}ms"
```

### Warning Alerts

```yaml
# Remote endpoint unhealthy
- alert: RemoteEndpointUnhealthy
  expr: venn_remote_health_status == 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Remote {{ $labels.remote }} for chain {{ $labels.chain }} is unhealthy"
    description: "Health checks are failing for {{ $labels.remote }}"

# Chain success rate low
- alert: ChainSuccessRateLow
  expr: venn_chain_request_success_rate < 95
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Chain {{ $labels.chain }} success rate low: {{ $value }}%"
    description: "Request success rate has dropped below 95%"

# High propagation delay
- alert: BlockPropagationSlow
  expr: venn_propagation_delay_ms > 5000
  for: 3m
  labels:
    severity: warning
  annotations:
    summary: "Block propagation delay high for {{ $labels.chain }}: {{ $value }}ms"
```

---

## Monitoring Dashboard

### Key Performance Indicators

1. **Chain Health Overview**
   - `venn_chain_availability_percent` (gauge panel)
   - `venn_chain_healthy_remote_count` vs `venn_chain_total_remote_count` (stat panel)
   - `venn_chain_request_success_rate` (gauge panel)

2. **Request Performance**
   - `rate(venn_request_latency_ms_sum[5m]) / rate(venn_request_latency_ms_count[5m])` (average latency)
   - `histogram_quantile(0.95, rate(venn_request_latency_ms_bucket[5m]))` (95th percentile)
   - `rate(venn_request_latency_ms_count{success="false"}[5m])` (error rate)

3. **Remote Endpoint Status**
   - `venn_remote_health_status` (status history or heatmap)
   - `rate(venn_remote_health_check_failures_total[5m])` (failure rate)
   - Remote latency comparison across providers

4. **Gateway Metrics**
   - `rate(venn_gateway_request_latency_ms_count[5m])` (requests per second)
   - Gateway latency percentiles
   - Subscription creation/closure rates

5. **Block Stalking** (if enabled)
   - `venn_stalker_head_block` (current block height)
   - `venn_propagation_delay_ms` (propagation delay trend)
   - Block propagation delay distribution

### Example PromQL Queries

```promql
# Average request latency by chain (5-minute rate)
rate(venn_request_latency_ms_sum[5m]) / rate(venn_request_latency_ms_count[5m])

# Request success rate by chain
rate(venn_request_latency_ms_count{success="true"}[5m]) / rate(venn_request_latency_ms_count[5m]) * 100

# Remote endpoint latency comparison
histogram_quantile(0.95, rate(venn_remote_latency_ms_bucket[5m])) by (chain, remote)

# Health check failure rate
rate(venn_remote_health_check_failures_total[5m])

# Time since last successful health check
time() - venn_remote_health_last_success_timestamp
```

---

## Implementation Details

- **Health Check Frequency**: Remote health checks run on adaptive intervals between min (5s) and max (30s) based on health status
- **Metric Update Frequency**: Chain-level metrics are aggregated every 30 seconds
- **Rolling Windows**: Success rate tracking uses 1-minute rolling windows for real-time accuracy
- **Health Check Logic**: Validates both `eth_blockNumber` (responsiveness) and `eth_chainId` (correct chain)
- **Stalker Operation**: Actively monitors block production and propagation delays for configured chains
- **Graceful Degradation**: Unhealthy remotes are excluded from request routing until they recover

All metrics integrate with the existing Venn Prometheus infrastructure and maintain backward compatibility.