// Package api holds the agent OpenAPI document (spec-first source of truth).
package api

import _ "embed"

// OpenAPIYAML is the agent API OpenAPI 3.1 document.
// The {{BASE_URL}} placeholder is replaced at serve time.
//
//go:embed openapi.yaml
var OpenAPIYAML string
