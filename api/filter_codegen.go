//go:build ignore

// filter_codegen drops chat-only operations (x-audiences present but without
// "mcp") so oapi-codegen does not invent HTTP ServerInterface methods for them.
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	inPath := "openapi.yaml"
	outPath := "openapi.http.yaml"
	if len(os.Args) >= 2 {
		inPath = os.Args[1]
	}
	if len(os.Args) >= 3 {
		outPath = os.Args[2]
	}
	raw, err := os.ReadFile(inPath)
	if err != nil {
		fail(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		fail(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		pathItem, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options"} {
			op, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			if dropForCodegen(op) {
				delete(pathItem, method)
			}
		}
		if !pathItemHasHTTPMethod(pathItem) {
			delete(paths, path)
		}
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		fail(err)
	}
}

func dropForCodegen(op map[string]any) bool {
	raw, ok := op["x-audiences"]
	if !ok || raw == nil {
		return false // unannotated → keep for HTTP/MCP
	}
	for _, a := range asStrings(raw) {
		if a == "mcp" {
			return false
		}
	}
	return true
}

func pathItemHasHTTPMethod(pathItem map[string]any) bool {
	for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options"} {
		if _, ok := pathItem[method]; ok {
			return true
		}
	}
	return false
}

func asStrings(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
