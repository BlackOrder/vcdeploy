// Package metrics provides Prometheus metrics for vcdeploy.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "vcdeploy"

var (
	// Deployment metrics
	DeploymentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "deployments_total",
			Help:      "Total number of deployments",
		},
		[]string{"project", "status"}, // status: success, failed, cancelled
	)

	DeploymentDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "deployment_duration_seconds",
			Help:      "Duration of deployments in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"project"},
	)

	DeploymentInProgress = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "deployments_in_progress",
			Help:      "Number of deployments currently in progress",
		},
		[]string{"project"},
	)

	// HTTP metrics
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "http_requests_in_flight",
			Help:      "Current number of HTTP requests being processed",
		},
	)

	// Agent metrics (aggregated from heartbeats)
	AgentConnected = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_connected",
			Help:      "Agent connection status (1=connected, 0=disconnected)",
		},
		[]string{"agent_id", "hostname"},
	)

	AgentCPUPercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_cpu_percent",
			Help:      "Agent CPU usage percentage",
		},
		[]string{"agent_id"},
	)

	AgentMemoryPercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_memory_percent",
			Help:      "Agent memory usage percentage",
		},
		[]string{"agent_id"},
	)

	AgentDiskPercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_disk_percent",
			Help:      "Agent disk usage percentage",
		},
		[]string{"agent_id"},
	)

	AgentsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agents_total",
			Help:      "Total number of registered agents",
		},
	)

	AgentsConnectedTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agents_connected_total",
			Help:      "Total number of currently connected agents",
		},
	)

	// gRPC metrics
	GRPCStreamDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "grpc_stream_duration_seconds",
			Help:      "Duration of gRPC streams",
			Buckets:   []float64{1, 5, 10, 30, 60, 300, 600, 1800},
		},
		[]string{"method"},
	)

	GRPCMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grpc_messages_received_total",
			Help:      "Total number of gRPC messages received",
		},
		[]string{"method"},
	)

	GRPCMessagesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grpc_messages_sent_total",
			Help:      "Total number of gRPC messages sent",
		},
		[]string{"method"},
	)

	// Database metrics
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_query_duration_seconds",
			Help:      "Database query duration in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"operation"}, // select, insert, update, delete
	)

	DBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "db_queries_total",
			Help:      "Total number of database queries",
		},
		[]string{"operation"},
	)

	// Webhook metrics
	WebhooksReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "webhooks_received_total",
			Help:      "Total number of webhooks received",
		},
		[]string{"provider", "event"}, // provider: github, gitlab, bitbucket
	)

	WebhooksProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "webhooks_processed_total",
			Help:      "Total number of webhooks processed",
		},
		[]string{"provider", "status"}, // status: success, ignored, error
	)

	// Secret access metrics
	SecretsAccessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "secrets_accessed_total",
			Help:      "Total number of secret accesses",
		},
		[]string{"operation"}, // read, write, delete
	)

	// Build info
	BuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_info",
			Help:      "Build information",
		},
		[]string{"version", "commit", "date"},
	)
)

// SetBuildInfo sets the build information metric.
func SetBuildInfo(version, commit, date string) {
	BuildInfo.WithLabelValues(version, commit, date).Set(1)
}
