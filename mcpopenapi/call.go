package mcpopenapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (op *operation) handler(upstream http.Handler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 && string(req.Params.Arguments) != "null" {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return toolErr(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}

		httpReq, err := op.buildRequest(ctx, args)
		if err != nil {
			return toolErr(err.Error()), nil
		}

		rec := httptest.NewRecorder()
		upstream.ServeHTTP(rec, httpReq)
		resp := rec.Result()
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))

		text := formatUpstreamResponse(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		result := &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
			IsError: resp.StatusCode >= 400,
		}
		if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "application/json") && len(body) > 0 {
			var structured any
			if json.Unmarshal(body, &structured) == nil {
				result.StructuredContent = structured
			}
		}
		return result, nil
	}
}

func (op *operation) buildRequest(ctx context.Context, args map[string]any) (*http.Request, error) {
	path := op.Path
	for _, p := range op.PathParams {
		raw, ok := args[p.Name]
		if !ok {
			return nil, fmt.Errorf("missing path parameter %q", p.Name)
		}
		seg := fmt.Sprint(raw)
		if seg == "" {
			return nil, fmt.Errorf("empty path parameter %q", p.Name)
		}
		path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(seg))
		delete(args, p.Name)
	}
	if strings.Contains(path, "{") {
		return nil, fmt.Errorf("unresolved path template: %s", path)
	}

	q := url.Values{}
	for _, p := range op.QueryParams {
		if raw, ok := args[p.Name]; ok {
			q.Set(p.Name, fmt.Sprint(raw))
			delete(args, p.Name)
		} else if p.Required {
			return nil, fmt.Errorf("missing query parameter %q", p.Name)
		}
	}

	header := http.Header{}
	for _, p := range op.HeaderParams {
		if !allowedUpstreamHeader(p.Name) {
			// Do not forward arbitrary/unknown headers to Upstream (confused-deputy).
			delete(args, p.Name)
			if p.Required {
				return nil, fmt.Errorf("unsupported header parameter %q", p.Name)
			}
			continue
		}
		if strings.EqualFold(p.Name, "Idempotency-Key") {
			if raw, ok := args[p.Name]; ok && fmt.Sprint(raw) != "" {
				header.Set("Idempotency-Key", fmt.Sprint(raw))
			} else {
				header.Set("Idempotency-Key", newIdempotencyKey())
			}
			delete(args, p.Name)
			continue
		}
		if raw, ok := args[p.Name]; ok {
			header.Set(p.Name, fmt.Sprint(raw))
			delete(args, p.Name)
		} else if p.Required {
			return nil, fmt.Errorf("missing header %q", p.Name)
		}
	}

	var bodyReader io.Reader
	switch {
	case op.Body == nil:
		// no body
	case op.Body.ContentType == "application/json":
		bodyObj := map[string]any{}
		if props := asMap(op.Body.Schema["properties"]); props != nil {
			for k := range props {
				if v, ok := args[k]; ok {
					bodyObj[k] = v
					delete(args, k)
				}
			}
		} else {
			for k, v := range args {
				bodyObj[k] = v
				delete(args, k)
			}
		}
		if op.Body.Required && len(bodyObj) == 0 {
			// Allow empty object when schema has no required fields.
		}
		raw, err := json.Marshal(bodyObj)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(raw)
		header.Set("Content-Type", "application/json")
	case op.Body.ContentType == "text/plain":
		name := op.Body.BodyProp
		if name == "" {
			name = "body"
		}
		raw, ok := args[name]
		if !ok {
			if op.Body.Required {
				return nil, fmt.Errorf("missing body argument %q", name)
			}
			raw = ""
		}
		delete(args, name)
		bodyReader = strings.NewReader(fmt.Sprint(raw))
		header.Set("Content-Type", "text/plain; charset=utf-8")
	}

	u := path
	if enc := q.Encode(); enc != "" {
		u = path + "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, op.Method, u, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header = header
	return req, nil
}

func formatUpstreamResponse(status int, contentType string, body []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "HTTP %d", status)
	if contentType != "" {
		fmt.Fprintf(&b, " (%s)", contentType)
	}
	b.WriteString("\n")
	if len(body) > 0 {
		b.Write(body)
	}
	return b.String()
}

func toolErr(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

func newIdempotencyKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// allowedUpstreamHeader is the allowlist of OpenAPI header params that may be
// copied onto the in-process Upstream request.
func allowedUpstreamHeader(name string) bool {
	return strings.EqualFold(name, "Idempotency-Key")
}
