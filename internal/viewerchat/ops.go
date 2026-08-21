package viewerchat

import "github.com/yaronf/tripmap/internal/tripops"

// Ops is the shared trip agent surface (same as HTTP/MCP).
type Ops = tripops.Ops

// Re-export result types used by tools and tests.
type (
	TripSummary  = tripops.TripSummary
	MutateResult = tripops.MutateResult
	VersionEntry = tripops.VersionEntry
	YAMLResult   = tripops.YAMLResult
)
