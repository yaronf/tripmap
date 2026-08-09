package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/pdfarchive"
	"github.com/yaronf/tripmap/internal/routebuild"
	"github.com/yaronf/tripmap/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tripmap", flag.ContinueOnError)
	in := fs.String("input", "trip.yaml", "input YAML itinerary (local file)")
	tripID := fs.String("trip", "", "load itinerary YAML + shared comments from S3 by trip id")
	notesPath := fs.String("notes", "", "optional local shared-comments JSON (with -input)")
	out := fs.String("output", "maps/trip.kml", "output KML file")
	pdfPath := fs.String("pdf", "", "write PDF archive to this path")
	bundleDir := fs.String("bundle", "", "write PWA bundle directory (trip.json, geo/, viewer)")
	routeMode := fs.String("route", "straight", "route mode: straight or osrm")
	simplify := fs.Float64("simplify", 0, "simplify OSRM route geometry (meters); 0 keeps full detail")
	precision := fs.Int("precision", 0, "decimal places for coordinates (default 6, or 5 with -mymaps)")
	mymaps := fs.Bool("mymaps", false, "optimize for Google My Maps (-simplify 100 -precision 5)")
	units := fs.String("units", "km", "distance units for PWA bundle / PDF: km or mi")
	if err := fs.Parse(args); err != nil {
		return err
	}

	outputSet, inputSet, tripSet := false, false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "output":
			outputSet = true
		case "input":
			inputSet = true
		case "trip":
			tripSet = true
		}
	})

	if tripSet && inputSet {
		return fmt.Errorf("use only one of -trip or -input")
	}
	if tripSet && *tripID == "" {
		return fmt.Errorf("-trip requires a trip id")
	}
	if *notesPath != "" && tripSet {
		return fmt.Errorf("-notes is only valid with -input (S3 notes load via -trip)")
	}
	if *notesPath != "" && *pdfPath == "" {
		return fmt.Errorf("-notes requires -pdf")
	}

	switch *units {
	case "km", "mi":
	default:
		return fmt.Errorf("invalid -units %q (use km or mi)", *units)
	}

	opts := routebuild.RouteOptions{Mode: *routeMode, SimplifyMeters: *simplify, CoordPrecision: *precision, Units: *units}
	if *mymaps {
		if opts.SimplifyMeters == 0 {
			opts.SimplifyMeters = 100
		}
		if opts.CoordPrecision == 0 {
			opts.CoordPrecision = 5
		}
		opts.Flatten = true
	}
	if opts.CoordPrecision == 0 {
		opts.CoordPrecision = 6
	}
	if (*bundleDir != "" || *pdfPath != "") && opts.Mode == "osrm" && opts.SimplifyMeters == 0 && !*mymaps {
		opts.SimplifyMeters = 100
		if opts.CoordPrecision > 5 {
			opts.CoordPrecision = 5
		}
	}

	ctx := context.Background()

	var (
		yamlBytes []byte
		notes     store.NotesDoc
		photoDir  string
		id        string
	)

	if tripSet {
		id = *tripID
		st, err := openS3Store()
		if err != nil {
			return err
		}
		obj, err := st.GetYAML(ctx, id)
		if err != nil {
			return fmt.Errorf("s3 yaml %q: %w", id, err)
		}
		yamlBytes = obj.Body
		if *pdfPath != "" {
			// Shared comments are optional; missing bucket / object → empty.
			if raw, err := st.GetNotes(ctx, id); err == nil && len(raw) > 0 {
				_ = json.Unmarshal(raw, &notes)
			}
		}
		if notes.Days == nil {
			notes.Days = map[string]string{}
		}
	} else {
		b, err := os.ReadFile(*in)
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		yamlBytes = b
		photoDir = filepath.Dir(*in)
		id = strings.TrimSuffix(filepath.Base(*in), filepath.Ext(*in))
		if *notesPath != "" {
			nb, err := os.ReadFile(*notesPath)
			if err != nil {
				return fmt.Errorf("read notes: %w", err)
			}
			if err := json.Unmarshal(nb, &notes); err != nil {
				return fmt.Errorf("parse notes: %w", err)
			}
		}
		if notes.Days == nil {
			notes.Days = map[string]string{}
		}
	}

	t, err := itinerary.ParseYAML(yamlBytes)
	if err != nil {
		return err
	}
	if err := itinerary.EnsureSchemaVersion(&t); err != nil {
		return err
	}
	if err := itinerary.ValidateBasic(t); err != nil {
		return err
	}
	if err := itinerary.ResolvePlaces(&t); err != nil {
		return err
	}
	if err := itinerary.ResolveDayDates(&t); err != nil {
		return err
	}

	if *pdfPath != "" {
		if err := pdfarchive.Build(ctx, t, notes, *pdfPath, pdfarchive.Options{
			Route:    opts,
			PhotoDir: photoDir,
		}); err != nil {
			return fmt.Errorf("pdf: %w", err)
		}
	}

	if *bundleDir != "" {
		if id == "" {
			id = "trip"
		}
		if err := bundle.Build(ctx, t, id, photoDir, *bundleDir, opts); err != nil {
			return fmt.Errorf("bundle: %w", err)
		}
	}

	skipKML := (*bundleDir != "" || *pdfPath != "") && !outputSet
	if skipKML {
		return nil
	}

	doc, err := buildDocument(ctx, t, opts)
	if err != nil {
		return err
	}

	outBytes, err := marshalKML(doc)
	if err != nil {
		return err
	}

	if err := os.WriteFile(*out, outBytes, 0644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func openS3Store() (store.Store, error) {
	bucket := strings.TrimSpace(os.Getenv("ITINERARIES_BUCKET"))
	if bucket == "" {
		return nil, fmt.Errorf("ITINERARIES_BUCKET is required with -trip")
	}
	region := envOr("AWS_REGION", "eu-central-1")
	awsCfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	return &store.S3{
		Client:         s3.NewFromConfig(awsCfg),
		Bucket:         bucket,
		CommentsBucket: strings.TrimSpace(os.Getenv("COMMENTS_BUCKET")),
	}, nil
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
