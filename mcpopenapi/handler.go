// Package mcpopenapi exposes an OpenAPI 3 document as MCP tools over
// Streamable HTTP, forwarding tools/call to an in-process Upstream handler.
package mcpopenapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config configures the OpenAPI→MCP bridge.
type Config struct {
	// Name and Version are advertised in the MCP initialize result.
	Name    string
	Version string
	// Instructions is server-wide guidance for clients (keep the first ~512
	// characters self-contained for Codex/ChatGPT).
	Instructions string
	// OpenAPIYAML is an OpenAPI 3.x document (YAML or JSON).
	OpenAPIYAML []byte
	// Upstream receives reconstructed HTTP requests for each tools/call.
	// Paths match the OpenAPI paths (e.g. /api/agent/trips). Auth is the
	// caller's responsibility (typically edge Bearer around this handler).
	Upstream http.Handler
	// PathPrefix, when non-empty, keeps only operations under that path
	// prefix (e.g. "/api/agent"). Empty means all operations with an
	// operationId except those with explicit empty security.
	PathPrefix string
}

// NewHandler builds a Streamable HTTP MCP handler (stateless) whose tools
// mirror OpenAPI operations.
func NewHandler(cfg Config) (http.Handler, error) {
	if cfg.Upstream == nil {
		return nil, fmt.Errorf("mcpopenapi: Upstream is required")
	}
	if len(cfg.OpenAPIYAML) == 0 {
		return nil, fmt.Errorf("mcpopenapi: OpenAPIYAML is required")
	}
	if cfg.Name == "" {
		cfg.Name = "openapi-mcp"
	}
	if cfg.Version == "" {
		cfg.Version = "0.0.0"
	}

	ops, err := parseOperations(cfg.OpenAPIYAML, cfg.PathPrefix)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("mcpopenapi: no operations found")
	}

	instructions := cfg.Instructions
	if instructions == "" {
		instructions = "Tools mirror the OpenAPI agent API. Prefer structured patch ops over raw YAML replace."
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    cfg.Name,
		Version: cfg.Version,
	}, &mcp.ServerOptions{
		Instructions: instructions,
	})

	for _, op := range ops {
		op := op
		server.AddTool(op.tool(), op.handler(cfg.Upstream))
	}

	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	}), nil
}

func toolDescription(summary, description string) string {
	summary = strings.TrimSpace(summary)
	description = strings.TrimSpace(description)
	switch {
	case summary != "" && description != "":
		if strings.HasPrefix(description, summary) {
			return description
		}
		return summary + ". " + description
	case summary != "":
		return summary
	default:
		return description
	}
}
