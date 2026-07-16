package httpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is runtime configuration for tripmapd.
type Config struct {
	Addr              string
	AgentBearerToken  string
	PublicBaseURL     string
	ItinerariesBucket string
	CommentsBucket    string
	AWSRegion         string
	MaxYAMLBytes      int64
	OSRMBaseURL       string
	RouteMode         string // straight | osrm
}

// LoadConfig reads configuration from the environment.
func LoadConfig() (Config, error) {
	cfg := Config{
		Addr:              envOr("ADDR", ":8080"),
		PublicBaseURL:     strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		ItinerariesBucket: os.Getenv("ITINERARIES_BUCKET"),
		CommentsBucket:    os.Getenv("COMMENTS_BUCKET"),
		AWSRegion:         envOr("AWS_REGION", "eu-central-1"),
		MaxYAMLBytes:      512 * 1024,
		OSRMBaseURL:       strings.TrimRight(os.Getenv("OSRM_BASE_URL"), "/"),
		RouteMode:         envOr("ROUTE_MODE", "osrm"),
	}
	if v := os.Getenv("MAX_YAML_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_YAML_BYTES: %w", err)
		}
		cfg.MaxYAMLBytes = n
	}

	token, err := resolveAgentToken()
	if err != nil {
		return Config{}, err
	}
	cfg.AgentBearerToken = token
	return cfg, nil
}

func resolveAgentToken() (string, error) {
	if t := strings.TrimSpace(os.Getenv("AGENT_BEARER_TOKEN")); t != "" {
		return t, nil
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_BEARER_SECRET_JSON")); raw != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return "", fmt.Errorf("AGENT_BEARER_SECRET_JSON: %w", err)
		}
		if t := strings.TrimSpace(m["token"]); t != "" {
			return t, nil
		}
		return "", fmt.Errorf("AGENT_BEARER_SECRET_JSON missing token key")
	}
	return "", fmt.Errorf("set AGENT_BEARER_TOKEN or AGENT_BEARER_SECRET_JSON")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
