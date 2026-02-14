// Package config provides configuration types and defaults for vcdeploy.
package config

import "time"

// Default values for security and service configuration.
// These can be overridden via configuration files.
const (
	// DefaultSessionTokenBytes is the default length of session tokens in bytes.
	DefaultSessionTokenBytes = 32

	// DefaultAPIKeyBytes is the default length of API keys in bytes.
	DefaultAPIKeyBytes = 32

	// DefaultCSRFTokenBytes is the default length of CSRF tokens in bytes.
	DefaultCSRFTokenBytes = 32

	// DefaultBcryptCost is the default bcrypt cost factor.
	// Higher values are more secure but slower.
	DefaultBcryptCost = 12

	// DefaultMaxQueryResults is the default maximum number of results per page.
	DefaultMaxQueryResults = 50

	// MaxQueryResultsLimit is the absolute maximum allowed query results.
	MaxQueryResultsLimit = 1000

	// DefaultHTTPPort is the default HTTP server port.
	DefaultHTTPPort = 9000

	// DefaultGRPCPort is the default gRPC server port.
	DefaultGRPCPort = 9001

	// DefaultKeepReleases is the default number of releases to keep during deployments.
	DefaultKeepReleases = 5

	// DefaultExecutionTimeout is the default timeout for deployment execution.
	DefaultExecutionTimeout = 10 * time.Minute

	// DefaultHTTPAddr is the default HTTP server address.
	DefaultHTTPAddr = ":9000"

	// DefaultGRPCAddr is the default gRPC server address.
	DefaultGRPCAddr = ":9001"
)
