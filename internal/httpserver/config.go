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
	Addr               string
	AgentBearerToken   string
	PublicBaseURL      string
	ItinerariesBucket  string
	CommentsBucket     string
	AWSRegion          string
	MaxYAMLBytes       int64
	OSRMBaseURL        string
	RouteMode          string // straight | osrm
	HelloClientID      string
	HelloClientSecret  string
	HelloRedirectURI   string
	HelloSessionSecret string
	HelloAllowedEmails []string // lowercase
	HelloAllowedSubs   []string
	OpenAIAPIKey       string
	OpenAIModel        string
	ChatAllowedEmails  []string // lowercase; subset of Hellō users allowed to chat
	ChatAllowedSubs    []string
}

// LoadConfig reads configuration from the environment.
func LoadConfig() (Config, error) {
	cfg := Config{
		Addr:               envOr("ADDR", ":8080"),
		PublicBaseURL:      strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		ItinerariesBucket:  os.Getenv("ITINERARIES_BUCKET"),
		CommentsBucket:     os.Getenv("COMMENTS_BUCKET"),
		AWSRegion:          envOr("AWS_REGION", "eu-central-1"),
		MaxYAMLBytes:       512 * 1024,
		OSRMBaseURL:        strings.TrimRight(os.Getenv("OSRM_BASE_URL"), "/"),
		RouteMode:          envOr("ROUTE_MODE", "osrm"),
		HelloClientID:      strings.TrimSpace(os.Getenv("HELLO_CLIENT_ID")),
		HelloClientSecret:  strings.TrimSpace(os.Getenv("HELLO_CLIENT_SECRET")),
		HelloRedirectURI:   strings.TrimSpace(os.Getenv("HELLO_REDIRECT_URI")),
		HelloSessionSecret: strings.TrimSpace(os.Getenv("HELLO_SESSION_SECRET")),
		OpenAIAPIKey:       resolveOpenAIAPIKey(),
		OpenAIModel:        envOr("OPENAI_MODEL", "gpt-4o"),
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

	if cfg.HelloClientID != "" && cfg.HelloSessionSecret == "" {
		return Config{}, fmt.Errorf("HELLO_SESSION_SECRET required when HELLO_CLIENT_ID is set")
	}
	if cfg.HelloClientID != "" {
		path := resolveAllowlistPath()
		emails, subs, err := loadHelloAllowlistFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("HELLO allowlist %s: %w", path, err)
		}
		cfg.HelloAllowedEmails = emails
		cfg.HelloAllowedSubs = subs
		if len(cfg.HelloAllowedEmails) == 0 && len(cfg.HelloAllowedSubs) == 0 {
			return Config{}, fmt.Errorf("HELLO allowlist %s is empty", path)
		}
	}
	if cfg.OpenAIAPIKey != "" {
		path := resolveChatAllowlistPath()
		emails, subs, err := loadHelloAllowlistFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("CHAT allowlist %s: %w", path, err)
		}
		cfg.ChatAllowedEmails = emails
		cfg.ChatAllowedSubs = subs
		if len(cfg.ChatAllowedEmails) == 0 && len(cfg.ChatAllowedSubs) == 0 {
			return Config{}, fmt.Errorf("CHAT allowlist %s is empty", path)
		}
	}
	return cfg, nil
}

func resolveOpenAIAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); k != "" {
		return k
	}
	if raw := strings.TrimSpace(os.Getenv("OPENAI_SECRET_JSON")); raw != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			if k := strings.TrimSpace(m["api_key"]); k != "" {
				return k
			}
		}
	}
	return ""
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
