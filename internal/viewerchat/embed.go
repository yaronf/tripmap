package viewerchat

import (
	_ "embed"
	"strings"
	"sync"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/yaronf/mcpopenapi"
	"github.com/yaronf/tripmap/api"
)

//go:embed prompt.txt
var systemPromptBase string

var (
	chatToolsOnce   sync.Once
	chatToolsCached []responses.ToolUnionParam
	chatToolsErr    error
)

func loadChatFunctionTools() ([]responses.ToolUnionParam, error) {
	schemas, err := mcpopenapi.ParseToolSchemasOpts([]byte(api.OpenAPIYAML), mcpopenapi.ParseOptions{
		Audience:           "chat",
		IncludeUnannotated: mcpopenapi.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	out := make([]responses.ToolUnionParam, 0, len(schemas))
	for _, s := range schemas {
		params := s.InputSchema
		if params == nil {
			params = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		} else {
			params = cloneSchemaMap(params)
			stripSessionBoundArgs(params)
		}
		desc := s.Description
		out = append(out, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        s.Name,
				Description: openai.String(desc),
				Parameters:  params,
				Strict:      openai.Bool(false),
			},
		})
	}
	return out, nil
}

func stripSessionBoundArgs(schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	if props != nil {
		delete(props, "id")
		delete(props, "Idempotency-Key")
	}
	switch req := schema["required"].(type) {
	case []string:
		filtered := req[:0]
		for _, r := range req {
			if r == "id" || r == "Idempotency-Key" {
				continue
			}
			filtered = append(filtered, r)
		}
		if len(filtered) == 0 {
			delete(schema, "required")
		} else {
			schema["required"] = filtered
		}
	case []any:
		filtered := make([]any, 0, len(req))
		for _, r := range req {
			s, _ := r.(string)
			if s == "id" || s == "Idempotency-Key" {
				continue
			}
			filtered = append(filtered, r)
		}
		if len(filtered) == 0 {
			delete(schema, "required")
		} else {
			schema["required"] = filtered
		}
	}
}

func cloneSchemaMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch t := v.(type) {
		case map[string]any:
			out[k] = cloneSchemaMap(t)
		case []any:
			cp := make([]any, len(t))
			copy(cp, t)
			out[k] = cp
		case []string:
			cp := make([]string, len(t))
			copy(cp, t)
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
}

func chatTools() []responses.ToolUnionParam {
	chatToolsOnce.Do(func() {
		fns, err := loadChatFunctionTools()
		if err != nil {
			chatToolsErr = err
			return
		}
		web := responses.ToolUnionParam{
			OfWebSearch: &responses.WebSearchToolParam{
				Type:              responses.WebSearchToolTypeWebSearch,
				SearchContextSize: responses.WebSearchToolSearchContextSizeLow,
			},
		}
		chatToolsCached = append([]responses.ToolUnionParam{web}, fns...)
	})
	if chatToolsErr != nil {
		panic("viewerchat tools: " + chatToolsErr.Error())
	}
	return chatToolsCached
}

func baseSystemPrompt() string {
	return strings.TrimSpace(systemPromptBase) + "\n"
}
