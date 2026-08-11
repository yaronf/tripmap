package api

import (
	"testing"
)

func TestAgentSchemaDocumentIncludesPlaceInfo(t *testing.T) {
	doc, err := AgentSchemaDocument()
	if err != nil {
		t.Fatal(err)
	}
	schemas, ok := doc["schemas"].(map[string]any)
	if !ok || schemas["PlaceInfo"] == nil || schemas["PlaceLogistics"] == nil || schemas["TripPatch"] == nil {
		t.Fatalf("schemas=%#v", doc["schemas"])
	}
	placeInfo, ok := schemas["PlaceInfo"].(map[string]any)
	if !ok {
		t.Fatalf("PlaceInfo type %T", schemas["PlaceInfo"])
	}
	if placeInfo["additionalProperties"] != false {
		t.Fatalf("PlaceInfo should set additionalProperties false: %#v", placeInfo["additionalProperties"])
	}
	props, _ := placeInfo["properties"].(map[string]any)
	if props["logistics"] == nil {
		t.Fatalf("PlaceInfo missing logistics: %#v", props)
	}
}
