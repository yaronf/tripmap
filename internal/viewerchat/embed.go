package viewerchat

import (
	_ "embed"
	"strings"
	"sync"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/yaronf/mcpopenapi"
)

//go:embed prompt.txt
var systemPromptBase string

//go:embed tools.openapi.yaml
var toolsOpenAPI []byte

var (
	chatToolsOnce   sync.Once
	chatToolsCached []responses.ToolUnionParam
	chatToolsErr    error
)

func loadChatFunctionTools() ([]responses.ToolUnionParam, error) {
	schemas, err := mcpopenapi.ParseToolSchemas(toolsOpenAPI, "")
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
