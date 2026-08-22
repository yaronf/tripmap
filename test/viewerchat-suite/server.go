package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/yaronf/tripmap/internal/httpserver"
	"github.com/yaronf/tripmap/internal/store"
)

type localServer struct {
	http *http.Server
	cfg  httpserver.Config
	ln   net.Listener
	mem  bool
}

func (s *localServer) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.http.Shutdown(ctx)
	_ = s.ln.Close()
}

func startLocalServer(ctx context.Context) (*localServer, string, error) {
	if os.Getenv("ROUTE_MODE") == "" {
		_ = os.Setenv("ROUTE_MODE", "straight")
	}
	if os.Getenv("HELLO_CLIENT_ID") == "" {
		_ = os.Setenv("HELLO_CLIENT_ID", "app_local_viewerchat_mt")
	}
	cfg, err := httpserver.LoadConfig()
	if err != nil {
		return nil, "", err
	}
	if cfg.OpenAIAPIKey == "" {
		return nil, "", fmt.Errorf("OPENAI_API_KEY or OPENAI_SECRET_JSON required")
	}
	if cfg.HelloSessionSecret == "" {
		return nil, "", fmt.Errorf("HELLO_SESSION_SECRET required")
	}
	if cfg.AgentBearerToken == "" {
		return nil, "", fmt.Errorf("AGENT_BEARER_TOKEN required")
	}
	cfg.PublicBaseURL = strings.TrimRight(cfg.PublicBaseURL, "/")

	var st store.Store
	mem := false
	if cfg.ItinerariesBucket == "" {
		fmt.Fprintln(os.Stderr, "ITINERARIES_BUCKET unset; using mem store (seed YAML from TRIPMAP_SEED_URL / prod)")
		st = store.NewMem()
		mem = true
	} else {
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.AWSRegion))
		if err != nil {
			return nil, "", fmt.Errorf("aws: %w", err)
		}
		st = &store.S3{
			Client:         s3.NewFromConfig(awsCfg),
			Bucket:         cfg.ItinerariesBucket,
			CommentsBucket: cfg.CommentsBucket,
		}
	}

	app := httpserver.New(cfg, st)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	httpSrv := &http.Server{
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = httpSrv.Serve(ln)
	}()
	baseURL := "http://" + ln.Addr().String()
	if cfg.PublicBaseURL == "" {
		cfg.PublicBaseURL = baseURL
	}
	return &localServer{http: httpSrv, cfg: cfg, ln: ln, mem: mem}, baseURL, nil
}
