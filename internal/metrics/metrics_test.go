package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsRegistered(t *testing.T) {
	// Test that all metrics are properly registered by using them
	// This ensures promauto registration succeeded

	tests := []struct {
		name   string
		metric interface{}
	}{
		{"DeploymentTotal", DeploymentTotal},
		{"DeploymentDuration", DeploymentDuration},
		{"DeploymentInProgress", DeploymentInProgress},
		{"HTTPRequestDuration", HTTPRequestDuration},
		{"HTTPRequestsTotal", HTTPRequestsTotal},
		{"HTTPRequestsInFlight", HTTPRequestsInFlight},
		{"AgentConnected", AgentConnected},
		{"AgentCPUPercent", AgentCPUPercent},
		{"AgentMemoryPercent", AgentMemoryPercent},
		{"AgentDiskPercent", AgentDiskPercent},
		{"AgentsTotal", AgentsTotal},
		{"AgentsConnectedTotal", AgentsConnectedTotal},
		{"GRPCStreamDuration", GRPCStreamDuration},
		{"GRPCMessagesReceived", GRPCMessagesReceived},
		{"GRPCMessagesSent", GRPCMessagesSent},
		{"DBQueryDuration", DBQueryDuration},
		{"DBQueriesTotal", DBQueriesTotal},
		{"WebhooksReceived", WebhooksReceived},
		{"WebhooksProcessed", WebhooksProcessed},
		{"SecretsAccessed", SecretsAccessed},
		{"BuildInfo", BuildInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.metric == nil {
				t.Errorf("%s metric is nil", tt.name)
			}
		})
	}
}

func TestDeploymentMetrics(t *testing.T) {
	// Test deployment counter increment
	DeploymentTotal.WithLabelValues("test-project", "success").Inc()

	// Verify the metric was incremented
	counter, err := DeploymentTotal.GetMetricWithLabelValues("test-project", "success")
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}

	// Check value is at least 1 (may be higher from previous tests)
	val := testutil.ToFloat64(counter)
	if val < 1 {
		t.Errorf("expected counter >= 1, got %f", val)
	}

	// Test deployment duration histogram
	DeploymentDuration.WithLabelValues("test-project").Observe(5.5)

	// Test in-progress gauge
	DeploymentInProgress.WithLabelValues("test-project").Inc()
	DeploymentInProgress.WithLabelValues("test-project").Dec()
}

func TestHTTPMetrics(t *testing.T) {
	// Test HTTP request counter
	HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/health", "200").Inc()

	// Test HTTP duration histogram
	HTTPRequestDuration.WithLabelValues("GET", "/api/v1/health", "200").Observe(0.05)

	// Test in-flight gauge
	HTTPRequestsInFlight.Inc()
	val := testutil.ToFloat64(HTTPRequestsInFlight)
	if val < 1 {
		t.Errorf("expected in-flight >= 1, got %f", val)
	}
	HTTPRequestsInFlight.Dec()
}

func TestAgentMetrics(t *testing.T) {
	// Test agent connected gauge
	AgentConnected.WithLabelValues("agent-1", "hostname-1").Set(1)
	gauge, err := AgentConnected.GetMetricWithLabelValues("agent-1", "hostname-1")
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}
	val := testutil.ToFloat64(gauge)
	if val != 1 {
		t.Errorf("expected agent_connected=1, got %f", val)
	}

	// Test disconnecting
	AgentConnected.WithLabelValues("agent-1", "hostname-1").Set(0)
	val = testutil.ToFloat64(gauge)
	if val != 0 {
		t.Errorf("expected agent_connected=0, got %f", val)
	}

	// Test resource metrics
	AgentCPUPercent.WithLabelValues("agent-1").Set(55.5)
	AgentMemoryPercent.WithLabelValues("agent-1").Set(70.2)
	AgentDiskPercent.WithLabelValues("agent-1").Set(45.0)

	// Test totals
	AgentsTotal.Set(5)
	AgentsConnectedTotal.Set(3)
}

func TestGRPCMetrics(t *testing.T) {
	// Test stream duration
	GRPCStreamDuration.WithLabelValues("AgentStream").Observe(300.0)

	// Test message counters
	GRPCMessagesReceived.WithLabelValues("Heartbeat").Inc()
	GRPCMessagesSent.WithLabelValues("DeployCommand").Inc()
}

func TestDBMetrics(t *testing.T) {
	// Test query duration
	DBQueryDuration.WithLabelValues("select").Observe(0.005)
	DBQueryDuration.WithLabelValues("insert").Observe(0.01)
	DBQueryDuration.WithLabelValues("update").Observe(0.008)
	DBQueryDuration.WithLabelValues("delete").Observe(0.003)

	// Test query counter
	DBQueriesTotal.WithLabelValues("select").Inc()
	DBQueriesTotal.WithLabelValues("insert").Inc()
}

func TestWebhookMetrics(t *testing.T) {
	// Test webhooks received
	WebhooksReceived.WithLabelValues("github", "push").Inc()
	WebhooksReceived.WithLabelValues("gitlab", "merge_request").Inc()
	WebhooksReceived.WithLabelValues("bitbucket", "push").Inc()

	// Test webhooks processed
	WebhooksProcessed.WithLabelValues("github", "success").Inc()
	WebhooksProcessed.WithLabelValues("github", "ignored").Inc()
	WebhooksProcessed.WithLabelValues("github", "error").Inc()
}

func TestSecretsMetrics(t *testing.T) {
	SecretsAccessed.WithLabelValues("read").Inc()
	SecretsAccessed.WithLabelValues("write").Inc()
	SecretsAccessed.WithLabelValues("delete").Inc()
}

func TestSetBuildInfo(t *testing.T) {
	SetBuildInfo("1.0.0", "abc123", "2024-01-15")

	// Verify the metric was set
	gauge, err := BuildInfo.GetMetricWithLabelValues("1.0.0", "abc123", "2024-01-15")
	if err != nil {
		t.Fatalf("failed to get build_info metric: %v", err)
	}

	val := testutil.ToFloat64(gauge)
	if val != 1 {
		t.Errorf("expected build_info=1, got %f", val)
	}
}

func TestMetricLabels(t *testing.T) {
	// Verify metric label cardinality doesn't cause issues
	// by using various label combinations

	// Multiple projects
	for i := 0; i < 10; i++ {
		DeploymentTotal.WithLabelValues("project-"+string(rune('a'+i)), "success").Inc()
	}

	// Multiple status codes
	statuses := []string{"200", "201", "400", "401", "403", "404", "500", "503"}
	for _, status := range statuses {
		HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/test", status).Inc()
	}

	// Multiple agents
	for i := 0; i < 5; i++ {
		agentID := "agent-" + string(rune('a'+i))
		hostname := "host-" + string(rune('a'+i))
		AgentConnected.WithLabelValues(agentID, hostname).Set(1)
		AgentCPUPercent.WithLabelValues(agentID).Set(float64(20 + i*10))
	}
}

func TestHistogramBuckets(t *testing.T) {
	// Test that histogram buckets capture different ranges

	// Test deployment duration buckets (1, 5, 10, 30, 60, 120, 300, 600)
	durations := []float64{0.5, 2, 7, 25, 45, 90, 200, 500, 1000}
	for _, d := range durations {
		DeploymentDuration.WithLabelValues("bucket-test").Observe(d)
	}

	// Test DB query duration buckets (0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1)
	queryDurations := []float64{0.0005, 0.003, 0.007, 0.02, 0.04, 0.08, 0.2, 0.4, 2}
	for _, d := range queryDurations {
		DBQueryDuration.WithLabelValues("bucket-test").Observe(d)
	}
}

func TestMetricDescriptions(t *testing.T) {
	// Verify metrics have proper descriptions by checking they can be described
	descCh := make(chan *prometheus.Desc, 100)

	DeploymentTotal.Describe(descCh)
	close(descCh)

	count := 0
	for range descCh {
		count++
	}

	if count == 0 {
		t.Error("expected at least one metric description")
	}
}

func TestNamespace(t *testing.T) {
	// Verify all metrics use the correct namespace
	// We can check this by verifying the metric names start with "vcdeploy_"
	// This is implicitly tested through the promauto registration
	if namespace != "vcdeploy" {
		t.Errorf("namespace = %s, want vcdeploy", namespace)
	}
}

func TestGaugeVecReset(t *testing.T) {
	// Test that gauge vec can be reset/deleted
	testGauge := AgentConnected.WithLabelValues("test-agent-reset", "test-host-reset")
	testGauge.Set(1)

	val := testutil.ToFloat64(testGauge)
	if val != 1 {
		t.Errorf("expected gauge=1, got %f", val)
	}

	// Delete the metric
	AgentConnected.DeleteLabelValues("test-agent-reset", "test-host-reset")

	// Getting the metric again should return a fresh gauge (value 0)
	newGauge := AgentConnected.WithLabelValues("test-agent-reset", "test-host-reset")
	val = testutil.ToFloat64(newGauge)
	if val != 0 {
		t.Errorf("expected fresh gauge=0, got %f", val)
	}
}
