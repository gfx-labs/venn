# Venn Prometheus Metrics

This document describes the Prometheus metrics exposed by Venn for monitoring remote endpoint health and chain availability.

## Remote Health Metrics

These metrics track the health of individual remote endpoints:

### `venn_remote_health_status`
**Type:** Gauge  
**Labels:** `chain`, `remote`  
**Description:** Health status of remote endpoint
- `1` = Healthy
- `0` = Unhealthy  
- `-1` = Unknown

**Example:**
```
venn_remote_health_status{chain="ethereum",remote="alchemy"} 1
venn_remote_health_status{chain="ethereum",remote="infura"} 0
```

### `venn_remote_health_check_latency_ms`
**Type:** Histogram  
**Labels:** `chain`, `remote`  
**Description:** Latency of health checks in milliseconds  
**Buckets:** 1, 10, 50, 100, 250, 500, 1000, 2000, 5000, 10000, 30000

### `venn_remote_health_check_failures_total`
**Type:** Counter  
**Labels:** `chain`, `remote`  
**Description:** Total number of health check failures

### `venn_remote_health_last_success_timestamp`
**Type:** Gauge  
**Labels:** `chain`, `remote`  
**Description:** Unix timestamp of last successful health check

## Chain Health Metrics

These metrics provide chain-level health overview:

### `venn_chain_availability_percent`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Percentage of healthy remotes for the chain (0-100)

**Example:**
```
venn_chain_availability_percent{chain="ethereum"} 66.67
```

### `venn_chain_healthy_remote_count`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Number of currently healthy remotes

### `venn_chain_total_remote_count`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Total number of configured remotes

### `venn_chain_request_success_rate`
**Type:** Gauge  
**Labels:** `chain`  
**Description:** Rolling average success rate of requests to the chain (0-100)

## Alerting Examples

### Critical Alerts

```yaml
# No healthy remotes for a chain
- alert: ChainNoHealthyRemotes
  expr: venn_chain_healthy_remote_count == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "No healthy remotes available for chain {{ $labels.chain }}"

# Chain availability below critical threshold
- alert: ChainAvailabilityCritical
  expr: venn_chain_availability_percent < 25
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Chain {{ $labels.chain }} availability critically low: {{ $value }}%"
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

# Chain success rate low
- alert: ChainSuccessRateLow
  expr: venn_chain_request_success_rate < 95
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Chain {{ $labels.chain }} success rate low: {{ $value }}%"
```

## Monitoring Dashboard

Key metrics to track on dashboards:

1. **Chain Health Overview**
   - `venn_chain_availability_percent` 
   - `venn_chain_healthy_remote_count`
   - `venn_chain_request_success_rate`

2. **Remote Endpoint Status**
   - `venn_remote_health_status` (as status panel)
   - `venn_remote_health_check_latency_ms` (as heatmap)

3. **Trends**
   - `rate(venn_remote_health_check_failures_total[5m])` (failure rate)
   - Health check latency percentiles

## Implementation Details

- Health metrics are updated automatically during health checks
- Chain-level metrics are updated every 30 seconds
- Success rate tracking uses a 1-minute rolling window
- All metrics integrate with existing Venn Prometheus infrastructure