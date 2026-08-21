package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func seedBaseURL() string {
	for _, k := range []string{"TRIPMAP_SEED_URL", "PUBLIC_BASE_URL"} {
		if v := strings.TrimRight(strings.TrimSpace(os.Getenv(k)), "/"); v != "" {
			return v
		}
	}
	return "https://tripmap.sheffer.org"
}

// setupTrip seeds the local store to the scenario restore_version.
// With S3, restores in-place. With mem store, fetches that version from
// TRIPMAP_SEED_URL (default prod) and create/put on the local server.
func setupTrip(ctx context.Context, localURL, bearer, tripID, versionID string, memStore bool) error {
	if !memStore {
		return restoreTrip(ctx, localURL, bearer, tripID, versionID)
	}
	yaml, err := fetchRemoteYAMLVersion(ctx, seedBaseURL(), bearer, tripID, versionID)
	if err != nil {
		return fmt.Errorf("fetch seed from %s: %w", seedBaseURL(), err)
	}
	return putLocalTripYAML(ctx, localURL, bearer, tripID, yaml)
}

func fetchRemoteYAMLVersion(ctx context.Context, remote, bearer, tripID, versionID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/agent/trips/%s/versions/%s", remote, tripID, versionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getVersion status=%d body=%s", res.StatusCode, truncate(string(b), 300))
	}
	return b, nil
}

func putLocalTripYAML(ctx context.Context, localURL, bearer, tripID string, yaml []byte) error {
	// Try create first (mem is empty); on conflict, PUT replace.
	createBody, _ := json.Marshal(map[string]string{
		"id":   tripID,
		"yaml": string(yaml),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, localURL+"/api/agent/trips", bytes.NewReader(createBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("viewerchat-mt-create-%d", time.Now().UnixNano()))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusOK {
		return nil
	}
	if res.StatusCode != http.StatusConflict {
		return fmt.Errorf("create trip status=%d body=%s", res.StatusCode, truncate(string(b), 300))
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPut, localURL+"/api/agent/trips/"+tripID+"/yaml", bytes.NewReader(yaml))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("viewerchat-mt-put-%d", time.Now().UnixNano()))
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ = io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("put yaml status=%d body=%s", res.StatusCode, truncate(string(b), 300))
	}
	return nil
}
