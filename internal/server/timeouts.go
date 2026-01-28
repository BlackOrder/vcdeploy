// Package server provides the master daemon HTTP and gRPC servers.
package server

import "time"

// Timeout constants for consistent timeout handling across the server.
// These constants define standard timeouts used in API handlers, middleware,
// and background tasks.
const (
	// TimeoutShort is for quick operations like simple DB reads.
	TimeoutShort = 5 * time.Second

	// TimeoutDefault is the default timeout for most API operations.
	TimeoutDefault = 10 * time.Second

	// TimeoutLong is for operations that may take longer, like batch updates.
	TimeoutLong = 30 * time.Second

	// TimeoutExtended is for operations like imports/exports or deployments.
	TimeoutExtended = 60 * time.Second

	// TimeoutCleanup is for background cleanup tasks.
	TimeoutCleanup = 5 * time.Minute

	// ThresholdStaleAgent is how long before an agent is considered stale.
	ThresholdStaleAgent = 5 * time.Minute

	// DurationRateLimitBlock is how long to block after rate limit exceeded.
	DurationRateLimitBlock = 15 * time.Minute

	// IntervalRateLimitCleanup is how often to clean up rate limit buckets.
	IntervalRateLimitCleanup = 5 * time.Minute

	// IntervalMiddlewareCleanup is how often security middleware runs cleanup.
	IntervalMiddlewareCleanup = 5 * time.Minute
)
