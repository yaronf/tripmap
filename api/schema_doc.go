package api

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// AgentSchemaNames are the OpenAPI component schemas agents need for patches.
var AgentSchemaNames = []string{
	"TripPatch",
	"UpdateDay",
	"PlacePatch",
	"DayPatch",
	"UpsertStop",
	"RemoveStop",
	"InsertDay",
	"PlaceInfo",
	"PlaceSource",
	"PlaceLink",
	"PlaceStats",
	"PlaceLogistics",
	"PlaceFacilities",
	"StopRef",
	"ReplaceDayRoutesRequest",
}

// AgentSchemaDocument returns schema_version metadata plus the named
// components/schemas entries from the embedded OpenAPI document (with $refs).
func AgentSchemaDocument() (map[string]any, error) {
	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal([]byte(OpenAPIYAML), &doc); err != nil {
		return nil, fmt.Errorf("parse openapi: %w", err)
	}
	if doc.Components.Schemas == nil {
		return nil, fmt.Errorf("openapi missing components.schemas")
	}
	schemas := make(map[string]any, len(AgentSchemaNames))
	for _, name := range AgentSchemaNames {
		s, ok := doc.Components.Schemas[name]
		if !ok {
			return nil, fmt.Errorf("openapi missing components.schemas.%s", name)
		}
		schemas[name] = s
	}
	return map[string]any{
		"schema_version": 2,
		"description":    "tripmap itinerary patch schemas from OpenAPI components (use $ref within schemas)",
		"notes_policy":   "Day and stop notes are human-authored. Do not modify them unless the user explicitly asks. Put enrichment in places.*.info (see PlaceInfo).",
		"replace_day_routes": "Use replaceDayRoutes (ReplaceDayRoutesRequest) for overnight/endpoint or full route changes — not upsert_stop.",
		"schemas":        schemas,
	}, nil
}
