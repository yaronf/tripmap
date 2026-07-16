package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/yaronf/tripmap/internal/httpserver"
	"github.com/yaronf/tripmap/internal/store"
)

func main() {
	cfg, err := httpserver.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	var st store.Store
	if cfg.ItinerariesBucket == "" {
		log.Printf("ITINERARIES_BUCKET unset; using in-memory store (dev only)")
		st = store.NewMem()
	} else {
		awsCfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(cfg.AWSRegion))
		if err != nil {
			log.Fatalf("aws config: %v", err)
		}
		st = &store.S3{
			Client:         s3.NewFromConfig(awsCfg),
			Bucket:         cfg.ItinerariesBucket,
			CommentsBucket: cfg.CommentsBucket,
		}
	}

	srv := httpserver.New(cfg, st)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("tripmapd listening on %s", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
