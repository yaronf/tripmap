package httpserver

import (
	"context"
	"errors"
	"testing"
)

func TestLoadConfigFromSecretARNs(t *testing.T) {
	t.Setenv("AGENT_BEARER_TOKEN", "")
	t.Setenv("AGENT_BEARER_SECRET_JSON", "")
	t.Setenv("AGENT_BEARER_SECRET_ARN", "arn:aws:secretsmanager:eu-central-1:123:secret:agent")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("HELLO_SESSION_SECRET", "")
	t.Setenv("SESSION_SECRET_ARN", "arn:aws:secretsmanager:eu-central-1:123:secret:session")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_SECRET_JSON", "")
	t.Setenv("OPENAI_SECRET_ARN", "")
	t.Setenv("DESCOPE_PROJECT_ID", "")

	prev := secretsFromAWS
	t.Cleanup(func() { secretsFromAWS = prev })
	secretsFromAWS = func(_ context.Context, secretID string) (string, error) {
		switch secretID {
		case "arn:aws:secretsmanager:eu-central-1:123:secret:agent":
			return `{"token":"arn-token"}`, nil
		case "arn:aws:secretsmanager:eu-central-1:123:secret:session":
			return `{"secret":"arn-session"}`, nil
		default:
			return "", errors.New("unknown secret")
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentBearerToken != "arn-token" {
		t.Fatalf("token = %q", cfg.AgentBearerToken)
	}
	if cfg.SessionSecret != "arn-session" {
		t.Fatalf("session = %q", cfg.SessionSecret)
	}
}

func TestResolveOpenAIFromARN(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_SECRET_JSON", "")
	t.Setenv("OPENAI_SECRET_ARN", "arn:aws:secretsmanager:eu-central-1:123:secret:openai")

	prev := secretsFromAWS
	t.Cleanup(func() { secretsFromAWS = prev })
	secretsFromAWS = func(_ context.Context, secretID string) (string, error) {
		if secretID == "arn:aws:secretsmanager:eu-central-1:123:secret:openai" {
			return `{"api_key":"sk-arn"}`, nil
		}
		return "", errors.New("unknown secret")
	}

	if got := resolveOpenAIAPIKey(); got != "sk-arn" {
		t.Fatalf("openai = %q", got)
	}
}

func TestEnvSecretsPreferOverARN(t *testing.T) {
	t.Setenv("AGENT_BEARER_TOKEN", "env-token")
	t.Setenv("AGENT_BEARER_SECRET_ARN", "arn:aws:secretsmanager:eu-central-1:123:secret:agent")
	t.Setenv("DESCOPE_PROJECT_ID", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_SECRET_JSON", "")
	t.Setenv("OPENAI_SECRET_ARN", "")

	prev := secretsFromAWS
	t.Cleanup(func() { secretsFromAWS = prev })
	called := false
	secretsFromAWS = func(context.Context, string) (string, error) {
		called = true
		return `{"token":"arn-token"}`, nil
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentBearerToken != "env-token" {
		t.Fatalf("token = %q", cfg.AgentBearerToken)
	}
	if called {
		t.Fatal("expected ARN fetch to be skipped when env token is set")
	}
}
