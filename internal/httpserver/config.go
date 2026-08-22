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
	Addr                   string
	AgentBearerToken       string
	PublicBaseURL          string
	ItinerariesBucket      string
	CommentsBucket         string
	AWSRegion              string
	MaxYAMLBytes           int64
	OSRMBaseURL            string
	RouteMode              string // straight | osrm
	DescopeProjectID       string
	SessionSecret          string
	AllowedEmails          []string // lowercase sign-in ACL (email only)
	AllowedSubs            []string // unused in v1; kept for CSV compat
	OpenAIAPIKey           string
	OpenAIModel            string
	ChatAllowedEmails      []string // lowercase; subset allowed to chat
	ChatAllowedSubs        []string // unused for chat ACL in v1
	ContactEmail           string   // privacy / legal contact (CONTACT_EMAIL)
	GoogleSiteVerification string   // Search Console HTML tag (GOOGLE_SITE_VERIFICATION)
}

// LoadConfig reads configuration from the environment.
func LoadConfig() (Config, error) {
	cfg := Config{
		Addr:                   envOr("ADDR", ":8080"),
		PublicBaseURL:          strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		ItinerariesBucket:      os.Getenv("ITINERARIES_BUCKET"),
		CommentsBucket:         os.Getenv("COMMENTS_BUCKET"),
		AWSRegion:              envOr("AWS_REGION", "eu-central-1"),
		MaxYAMLBytes:           512 * 1024,
		OSRMBaseURL:            strings.TrimRight(os.Getenv("OSRM_BASE_URL"), "/"),
		RouteMode:              envOr("ROUTE_MODE", "osrm"),
		DescopeProjectID:       strings.TrimSpace(os.Getenv("DESCOPE_PROJECT_ID")),
		SessionSecret:          resolveSessionSecret(),
		OpenAIAPIKey:           resolveOpenAIAPIKey(),
		OpenAIModel:            envOr("OPENAI_MODEL", "gpt-5-mini"),
		ContactEmail:           strings.TrimSpace(os.Getenv("CONTACT_EMAIL")),
		GoogleSiteVerification: strings.TrimSpace(os.Getenv("GOOGLE_SITE_VERIFICATION")),
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

	if cfg.DescopeProjectID != "" && cfg.SessionSecret == "" {
		return Config{}, fmt.Errorf("SESSION_SECRET required when DESCOPE_PROJECT_ID is set")
	}
	if cfg.DescopeProjectID != "" {
		path := resolveUsersPath()
		users, err := loadUsersFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("users file %s: %w", path, err)
		}
		cfg.AllowedEmails = users.LoginEmails
		cfg.AllowedSubs = users.LoginSubs
		if len(cfg.AllowedEmails) == 0 {
			return Config{}, fmt.Errorf("users file %s has no sign-in email rows", path)
		}
		cfg.ChatAllowedEmails = users.ChatEmails
		cfg.ChatAllowedSubs = users.ChatSubs
	}
	if cfg.OpenAIAPIKey != "" {
		if cfg.DescopeProjectID == "" {
			path := resolveUsersPath()
			users, err := loadUsersFile(path)
			if err != nil {
				return Config{}, fmt.Errorf("users file %s: %w", path, err)
			}
			cfg.ChatAllowedEmails = users.ChatEmails
			cfg.ChatAllowedSubs = users.ChatSubs
		}
		if len(cfg.ChatAllowedEmails) == 0 && len(cfg.ChatAllowedSubs) == 0 {
			return Config{}, fmt.Errorf("users file %s has no chat=yes rows (required when OpenAI is configured)", resolveUsersPath())
		}
	}
	return cfg, nil
}

func resolveSessionSecret() string {
	if s := strings.TrimSpace(os.Getenv("SESSION_SECRET")); s != "" {
		return s
	}
	return strings.TrimSpace(os.Getenv("HELLO_SESSION_SECRET"))
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
