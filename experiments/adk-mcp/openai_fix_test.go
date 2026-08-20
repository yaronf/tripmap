package main

import (
	"encoding/json"
	"testing"
)

func TestRewriteAssistantInputText(t *testing.T) {
	in := []byte(`{
	  "model":"gpt-5-mini",
	  "input":[
	    {"role":"user","content":[{"type":"input_text","text":"hi"}],"type":"message"},
	    {"role":"assistant","content":[{"type":"input_text","text":"hello"}],"type":"message"},
	    {"role":"user","content":[{"type":"input_text","text":"again"}],"type":"message"}
	  ]
	}`)
	out, changed := rewriteAssistantInputText(in)
	if !changed {
		t.Fatal("expected rewrite")
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	items := root["input"].([]any)
	asst := items[1].(map[string]any)
	block := asst["content"].([]any)[0].(map[string]any)
	if block["type"] != "output_text" {
		t.Fatalf("assistant type = %v, want output_text", block["type"])
	}
	user := items[0].(map[string]any)
	ub := user["content"].([]any)[0].(map[string]any)
	if ub["type"] != "input_text" {
		t.Fatalf("user type = %v, want input_text", ub["type"])
	}
}
