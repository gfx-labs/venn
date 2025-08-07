package prom_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/svc/shared/services/prom"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRemoteHealthMetrics(t *testing.T) {
	// Create a logger that discards output for testing
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a doctor to test metrics integration
	doctor := callcenter.NewDoctor(logger, 1, "test-chain", "test-remote", 5*time.Second, 30*time.Second)

	// Verify doctor has correct metadata
	if doctor.GetChainName() != "test-chain" {
		t.Errorf("Expected chain name 'test-chain', got '%s'", doctor.GetChainName())
	}

	if doctor.GetRemoteName() != "test-remote" {
		t.Errorf("Expected remote name 'test-remote', got '%s'", doctor.GetRemoteName())
	}

	// Test that we can update remote health metrics
	healthLabel := prom.RemoteHealthLabel{
		Chain:  "test-chain",
		Remote: "test-remote",
	}

	// Reset metrics for this test
	prom.RemoteHealth.Status(healthLabel).Set(0)
	prom.RemoteHealth.CheckFailures(healthLabel).Add(0) // Reset counter

	// Set health status to healthy
	prom.RemoteHealth.Status(healthLabel).Set(1)

	// Record some latency measurements
	prom.RemoteHealth.CheckLatency(healthLabel).Observe(150)
	prom.RemoteHealth.CheckLatency(healthLabel).Observe(200)
	prom.RemoteHealth.CheckLatency(healthLabel).Observe(100)

	// Increment failure counter
	prom.RemoteHealth.CheckFailures(healthLabel).Inc()
	prom.RemoteHealth.CheckFailures(healthLabel).Inc()

	// Set last success timestamp
	now := time.Now().Unix()
	prom.RemoteHealth.LastSuccessTimestamp(healthLabel).Set(float64(now))

	// Verify the health status metric has the correct value
	if value := testutil.ToFloat64(prom.RemoteHealth.Status(healthLabel)); value != 1 {
		t.Errorf("Expected health status to be 1, got %f", value)
	}

	// Verify the timestamp metric is set to approximately now
	if value := testutil.ToFloat64(prom.RemoteHealth.LastSuccessTimestamp(healthLabel)); value < float64(now-1) || value > float64(now+1) {
		t.Errorf("Expected timestamp to be around %d, got %f", now, value)
	}

	// Test setting health status to unhealthy
	prom.RemoteHealth.Status(healthLabel).Set(0)
	if value := testutil.ToFloat64(prom.RemoteHealth.Status(healthLabel)); value != 0 {
		t.Errorf("Expected health status to be 0, got %f", value)
	}

	// Test setting health status to unknown
	prom.RemoteHealth.Status(healthLabel).Set(-1)
	if value := testutil.ToFloat64(prom.RemoteHealth.Status(healthLabel)); value != -1 {
		t.Errorf("Expected health status to be -1, got %f", value)
	}
}

func TestChainHealthMetrics(t *testing.T) {
	// Test chain health metrics with specific values
	chainLabel := prom.ChainHealthLabel{
		Chain: "test-chain",
	}

	// Update chain metrics with test values
	prom.ChainHealth.HealthyRemoteCount(chainLabel).Set(2)
	prom.ChainHealth.TotalRemoteCount(chainLabel).Set(3)
	prom.ChainHealth.AvailabilityPercent(chainLabel).Set(66.67)
	prom.ChainHealth.RequestSuccessRate(chainLabel).Set(95.5)

	// Verify each metric has the correct value
	if value := testutil.ToFloat64(prom.ChainHealth.HealthyRemoteCount(chainLabel)); value != 2 {
		t.Errorf("Expected healthy remote count to be 2, got %f", value)
	}

	if value := testutil.ToFloat64(prom.ChainHealth.TotalRemoteCount(chainLabel)); value != 3 {
		t.Errorf("Expected total remote count to be 3, got %f", value)
	}

	if value := testutil.ToFloat64(prom.ChainHealth.AvailabilityPercent(chainLabel)); value != 66.67 {
		t.Errorf("Expected availability percent to be 66.67, got %f", value)
	}

	if value := testutil.ToFloat64(prom.ChainHealth.RequestSuccessRate(chainLabel)); value != 95.5 {
		t.Errorf("Expected request success rate to be 95.5, got %f", value)
	}

	// Test edge cases
	prom.ChainHealth.HealthyRemoteCount(chainLabel).Set(0)
	prom.ChainHealth.AvailabilityPercent(chainLabel).Set(0)

	if value := testutil.ToFloat64(prom.ChainHealth.HealthyRemoteCount(chainLabel)); value != 0 {
		t.Errorf("Expected healthy remote count to be 0, got %f", value)
	}

	if value := testutil.ToFloat64(prom.ChainHealth.AvailabilityPercent(chainLabel)); value != 0 {
		t.Errorf("Expected availability percent to be 0, got %f", value)
	}
}

func TestCollectorSuccessTracking(t *testing.T) {
	// Test that the collector can track success rates
	collector := callcenter.NewCollector("test-chain", "test-remote")

	if collector.GetChainName() != "test-chain" {
		t.Errorf("Expected chain name 'test-chain', got '%s'", collector.GetChainName())
	}

	// Initially should have 100% success rate (no requests)
	if rate := collector.GetSuccessRate(); rate != 100.0 {
		t.Errorf("Expected initial success rate to be 100.0, got %f", rate)
	}

	// Initially should have 0 requests per minute
	if rpm := collector.GetRequestsPerMinute(); rpm != 0.0 {
		t.Errorf("Expected initial RPM to be 0.0, got %f", rpm)
	}
}

func TestRemoteMetrics(t *testing.T) {
	// Test remote latency metrics
	remoteLabel := prom.RemoteLabel{
		Chain:   "ethereum",
		Remote:  "alchemy",
		Method:  "eth_getBalance",
		Success: true,
	}

	// Record some latency measurements
	prom.Remotes.Latency(remoteLabel).Observe(100)
	prom.Remotes.Latency(remoteLabel).Observe(150)
	prom.Remotes.Latency(remoteLabel).Observe(200)

	// Test failed request
	failedLabel := prom.RemoteLabel{
		Chain:   "ethereum",
		Remote:  "alchemy",
		Method:  "eth_getBalance",
		Success: false,
	}
	prom.Remotes.Latency(failedLabel).Observe(5000) // Failed requests often take longer
}

func TestRequestMetrics(t *testing.T) {
	// Test request latency metrics
	requestLabel := prom.RequestLabel{
		Chain:   "ethereum",
		Method:  "eth_blockNumber",
		Success: true,
	}

	// Record some latency measurements
	prom.Requests.Latency(requestLabel).Observe(50)
	prom.Requests.Latency(requestLabel).Observe(75)
	prom.Requests.Latency(requestLabel).Observe(100)
}

func TestGatewayMetrics(t *testing.T) {
	// Test gateway metrics
	gatewayLabel := prom.GatewayRequestLabel{
		Endpoint: "api",
		Target:   "ethereum",
		Method:   "eth_getBalance",
		Success:  true,
	}

	// Record request latency
	prom.Gateway.RequestLatency(gatewayLabel).Observe(120)

	// Test subscription metrics
	subLabel := prom.GatewayRequestLabel{
		Endpoint: "api",
		Target:   "ethereum",
		Method:   "eth_subscribe",
		Success:  true,
	}

	// Increment subscription counters
	prom.Gateway.SubscriptionCreated(subLabel).Inc()
	prom.Gateway.SubscriptionClosed(subLabel).Inc()
}

func TestStalkerMetrics(t *testing.T) {
	// Test stalker metrics
	stalkerLabel := prom.StalkerLabel{
		Chain: "ethereum",
	}

	// Update stalker metrics
	prom.Stalker.PropagationDelayMean(stalkerLabel).Set(150)
	prom.Stalker.HeadBlock(stalkerLabel).Set(18500000)

	// Record block propagation delay measurements
	prom.Stalker.BlockPropagationDelay(stalkerLabel).Observe(100)
	prom.Stalker.BlockPropagationDelay(stalkerLabel).Observe(200)
	prom.Stalker.BlockPropagationDelay(stalkerLabel).Observe(300)

	// Verify metrics have correct values
	if value := testutil.ToFloat64(prom.Stalker.PropagationDelayMean(stalkerLabel)); value != 150 {
		t.Errorf("Expected propagation delay mean to be 150, got %f", value)
	}

	if value := testutil.ToFloat64(prom.Stalker.HeadBlock(stalkerLabel)); value != 18500000 {
		t.Errorf("Expected head block to be 18500000, got %f", value)
	}
}

func TestMetricsIntegration(t *testing.T) {
	// Test that all metrics can be gathered without errors
	gatherer := prometheus.DefaultGatherer
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that we have metrics
	if len(metricFamilies) == 0 {
		t.Error("No metrics were gathered")
	}

	// Verify we can find our key metric families
	expectedMetrics := []string{
		"venn_remote_health_status",
		"venn_chain_healthy_remote_count",
		"venn_remote_latency_ms",
		"venn_request_latency_ms",
		"venn_gateway_request_latency_ms",
		"venn_stalker_head_block",
	}

	foundMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		for _, expected := range expectedMetrics {
			if mf.GetName() == expected {
				foundMetrics[expected] = true
			}
		}
	}

	for _, expected := range expectedMetrics {
		if !foundMetrics[expected] {
			t.Errorf("Expected metric %s not found", expected)
		}
	}
}
