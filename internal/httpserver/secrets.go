package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// secretGetter fetches a Secrets Manager secret string by ARN or name.
// Tests inject a fake; production uses the AWS SDK.
type secretGetter func(ctx context.Context, secretID string) (string, error)

// secretsFromAWS is overridden in tests.
var secretsFromAWS secretGetter = fetchSecretString

func fetchSecretString(ctx context.Context, secretID string) (string, error) {
	region := envOr("AWS_REGION", "eu-central-1")
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("aws config: %w", err)
	}
	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return "", err
	}
	if out.SecretString == nil {
		return "", fmt.Errorf("secret %s has no SecretString", secretID)
	}
	return *out.SecretString, nil
}

func secretStringFromARN(arnEnv string) (string, error) {
	arn := strings.TrimSpace(os.Getenv(arnEnv))
	if arn == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := secretsFromAWS(ctx, arn)
	if err != nil {
		return "", fmt.Errorf("%s (%s): %w", arnEnv, arn, err)
	}
	return raw, nil
}

func jsonField(raw, key string) (string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", err
	}
	return strings.TrimSpace(m[key]), nil
}
