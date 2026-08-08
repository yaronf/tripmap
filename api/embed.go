// Package api holds the agent OpenAPI document (spec-first source of truth).
package api

import _ "embed"

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config=oapi-codegen.yaml openapi.yaml

// OpenAPIYAML is the agent API OpenAPI 3.1 document.
// The {{BASE_URL}} placeholder is replaced at serve time.
//
//go:embed openapi.yaml
var OpenAPIYAML string
