package mcpopenapi

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

type operation struct {
	ID          string
	Method      string
	Path        string
	Summary     string
	Description string
	PathParams  []param
	QueryParams []param
	HeaderParams []param
	Body        *bodySpec
	InputSchema map[string]any
	ReadOnly    bool
}

type param struct {
	Name     string
	Required bool
	Schema   map[string]any
}

type bodySpec struct {
	Required    bool
	ContentType string
	// Schema is the JSON schema for application/json bodies (object).
	Schema map[string]any
	// For non-object bodies (e.g. text/plain), BodyProp is the tool arg name.
	BodyProp string
}

func parseOperations(raw []byte, pathPrefix string) ([]*operation, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse openapi: %w", err)
	}
	paths, _ := doc["paths"].(map[string]any)
	if paths == nil {
		return nil, fmt.Errorf("openapi: missing paths")
	}

	var ops []*operation
	for path, item := range paths {
		pathItem, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if pathPrefix != "" && !strings.HasPrefix(path, pathPrefix) {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			opNode, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			if skipOperation(opNode) {
				continue
			}
			opID, _ := opNode["operationId"].(string)
			if opID == "" {
				continue
			}
			op, err := buildOperation(doc, opID, strings.ToUpper(method), path, opNode)
			if err != nil {
				return nil, fmt.Errorf("operation %s: %w", opID, err)
			}
			ops = append(ops, op)
		}
	}
	// Stable order for tests / listTools.
	sortOperations(ops)
	return ops, nil
}

func skipOperation(opNode map[string]any) bool {
	sec, ok := opNode["security"]
	if !ok {
		return false
	}
	arr, ok := sec.([]any)
	if !ok {
		return false
	}
	// Explicit empty security means public — skip for agent MCP.
	return len(arr) == 0
}

func buildOperation(doc map[string]any, id, method, path string, opNode map[string]any) (*operation, error) {
	op := &operation{
		ID:          id,
		Method:      method,
		Path:        path,
		Summary:     asString(opNode["summary"]),
		Description: asString(opNode["description"]),
		ReadOnly:    method == "GET" || method == "HEAD",
	}

	for _, p := range asSlice(opNode["parameters"]) {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		name := asString(pm["name"])
		in := asString(pm["in"])
		req, _ := pm["required"].(bool)
		schema := resolveSchema(doc, asMap(pm["schema"]))
		if schema == nil {
			schema = map[string]any{"type": "string"}
		}
		par := param{Name: name, Required: req, Schema: schema}
		switch in {
		case "path":
			op.PathParams = append(op.PathParams, par)
		case "query":
			op.QueryParams = append(op.QueryParams, par)
		case "header":
			op.HeaderParams = append(op.HeaderParams, par)
		}
	}

	if rb, ok := opNode["requestBody"].(map[string]any); ok {
		body, err := parseRequestBody(doc, rb)
		if err != nil {
			return nil, err
		}
		op.Body = body
	}

	op.InputSchema = buildInputSchema(op)
	return op, nil
}

func parseRequestBody(doc map[string]any, rb map[string]any) (*bodySpec, error) {
	required, _ := rb["required"].(bool)
	content := asMap(rb["content"])
	if content == nil {
		return nil, fmt.Errorf("requestBody missing content")
	}
	if appJSON, ok := content["application/json"].(map[string]any); ok {
		schema := resolveSchema(doc, asMap(appJSON["schema"]))
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		return &bodySpec{
			Required:    required,
			ContentType: "application/json",
			Schema:      schema,
		}, nil
	}
	if textPlain, ok := content["text/plain"].(map[string]any); ok {
		schema := resolveSchema(doc, asMap(textPlain["schema"]))
		prop := map[string]any{"type": "string"}
		if schema != nil {
			if t := asString(schema["type"]); t != "" {
				prop["type"] = t
			}
			if d := asString(schema["description"]); d != "" {
				prop["description"] = d
			}
		}
		return &bodySpec{
			Required:    required,
			ContentType: "text/plain",
			BodyProp:    "body",
			Schema:      prop,
		}, nil
	}
	return nil, fmt.Errorf("unsupported requestBody content types")
}

func buildInputSchema(op *operation) map[string]any {
	props := map[string]any{}
	required := []string{}

	addParam := func(p param, forceOptional bool) {
		schema := cloneMap(p.Schema)
		if schema == nil {
			schema = map[string]any{"type": "string"}
		}
		props[p.Name] = schema
		if p.Required && !forceOptional {
			required = append(required, p.Name)
		}
	}

	for _, p := range op.PathParams {
		addParam(p, false)
	}
	for _, p := range op.QueryParams {
		addParam(p, false)
	}
	for _, p := range op.HeaderParams {
		// Auto-filled when omitted.
		optional := strings.EqualFold(p.Name, "Idempotency-Key")
		addParam(p, optional)
	}

	if op.Body != nil {
		switch op.Body.ContentType {
		case "application/json":
			bodyProps := asMap(op.Body.Schema["properties"])
			for k, v := range bodyProps {
				if _, exists := props[k]; exists {
					continue
				}
				props[k] = cloneAny(v)
			}
			for _, r := range asStringSlice(op.Body.Schema["required"]) {
				if _, exists := props[r]; exists {
					required = append(required, r)
				}
			}
		case "text/plain":
			name := op.Body.BodyProp
			if name == "" {
				name = "body"
			}
			props[name] = cloneMap(op.Body.Schema)
			if op.Body.Required {
				required = append(required, name)
			}
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = uniqueStrings(required)
	}
	return schema
}

func (op *operation) tool() *mcp.Tool {
	t := &mcp.Tool{
		Name:        op.ID,
		Description: toolDescription(op.Summary, op.Description),
		InputSchema: op.InputSchema,
	}
	if op.ReadOnly {
		t.Annotations = &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        op.Summary,
		}
	} else if op.Summary != "" {
		t.Annotations = &mcp.ToolAnnotations{Title: op.Summary}
	}
	return t
}

func resolveSchema(doc map[string]any, schema map[string]any) map[string]any {
	return resolveSchemaDepth(doc, schema, 0)
}

func resolveSchemaDepth(doc map[string]any, schema map[string]any, depth int) map[string]any {
	if schema == nil || depth > 32 {
		return schema
	}
	if ref := asString(schema["$ref"]); ref != "" {
		target := lookupRef(doc, ref)
		if target == nil {
			return map[string]any{"type": "object", "properties": map[string]any{}}
		}
		return resolveSchemaDepth(doc, target, depth+1)
	}
	out := cloneMap(schema)
	if props := asMap(out["properties"]); props != nil {
		np := map[string]any{}
		for k, v := range props {
			if vm := asMap(v); vm != nil {
				np[k] = resolveSchemaDepth(doc, vm, depth+1)
			} else {
				np[k] = cloneAny(v)
			}
		}
		out["properties"] = np
	}
	if items := asMap(out["items"]); items != nil {
		out["items"] = resolveSchemaDepth(doc, items, depth+1)
	}
	if ap := asMap(out["additionalProperties"]); ap != nil {
		out["additionalProperties"] = resolveSchemaDepth(doc, ap, depth+1)
	}
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		if arr := asSlice(out[key]); len(arr) > 0 {
			na := make([]any, len(arr))
			for i, v := range arr {
				if vm := asMap(v); vm != nil {
					na[i] = resolveSchemaDepth(doc, vm, depth+1)
				} else {
					na[i] = cloneAny(v)
				}
			}
			out[key] = na
		}
	}
	return out
}

func lookupRef(doc map[string]any, ref string) map[string]any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	cur := any(doc)
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return asMap(cur)
}

func sortOperations(ops []*operation) {
	for i := 0; i < len(ops); i++ {
		for j := i + 1; j < len(ops); j++ {
			if ops[j].ID < ops[i].ID {
				ops[i], ops[j] = ops[j], ops[i]
			}
		}
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func asStringSlice(v any) []string {
	arr := asSlice(v)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneAny(v)
	}
	return out
}

func cloneAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneMap(t)
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = cloneAny(x)
		}
		return out
	default:
		return v
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
