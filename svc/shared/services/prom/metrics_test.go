package prom_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"gfx.cafe/gfx/venn/lib/callcenter"
	"gfx.cafe/gfx/venn/svc/shared/services/prom"
	"github.com/prometheus/client_golang/prometheus"
)

func TestRemoteHealthMetrics(t *testing.T) {
	// Create a logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

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

	// Set health status
	prom.RemoteHealth.Status(healthLabel).Set(1) // Healthy

	// Record some latency
	prom.RemoteHealth.CheckLatency(healthLabel).Observe(150)

	// Increment failure counter
	prom.RemoteHealth.CheckFailures(healthLabel).Inc()

	// Set last success timestamp
	now := time.Now().Unix()
	prom.RemoteHealth.LastSuccessTimestamp(healthLabel).Set(float64(now))

	// Verify metrics can be gathered (basic sanity check)
	gatherer := prometheus.DefaultGatherer
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that we have some metrics
	if len(metricFamilies) == 0 {
		t.Error("No metrics were gathered")
	}

	// Look for our specific metrics
	foundRemoteHealth := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "venn_remote_health_status" {
			foundRemoteHealth = true
			break
		}
	}

	if !foundRemoteHealth {
		t.Error("Remote health status metric not found")
	}
}

func TestChainHealthMetrics(t *testing.T) {
	// Test chain health metrics
	chainLabel := prom.ChainHealthLabel{
		Chain: "test-chain",
	}

	// Update chain metrics
	prom.ChainHealth.HealthyRemoteCount(chainLabel).Set(2)
	prom.ChainHealth.TotalRemoteCount(chainLabel).Set(3)
	prom.ChainHealth.AvailabilityPercent(chainLabel).Set(66.67)
	prom.ChainHealth.RequestSuccessRate(chainLabel).Set(95.5)

	// Verify metrics can be gathered (basic sanity check)
	gatherer := prometheus.DefaultGatherer
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Look for our chain health metrics
	foundChainHealth := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "venn_chain_healthy_remote_count" {
			foundChainHealth = true
			break
		}
	}

	if !foundChainHealth {
		t.Error("Chain health metric not found")
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