package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tripmap", flag.ContinueOnError)
	in := fs.String("input", "itineraries/holland.yaml", "input YAML itinerary")
	out := fs.String("output", "maps/holland.kml", "output KML file")
	bundleDir := fs.String("bundle", "", "write PWA bundle directory (trip.json, geo/, viewer)")
	routeMode := fs.String("route", "straight", "route mode: straight or osrm")
	simplify := fs.Float64("simplify", 0, "simplify OSRM route geometry (meters); 0 keeps full detail")
	precision := fs.Int("precision", 0, "decimal places for coordinates (default 6, or 5 with -mymaps)")
	mymaps := fs.Bool("mymaps", false, "optimize for Google My Maps (-simplify 100 -precision 5)")
	units := fs.String("units", "km", "distance units for PWA bundle: km or mi")
	if err := fs.Parse(args); err != nil {
		return err
	}

	outputSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "output" {
			outputSet = true
		}
	})

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
	if *bundleDir != "" && opts.Mode == "osrm" && opts.SimplifyMeters == 0 && !*mymaps {
		opts.SimplifyMeters = 100
		if opts.CoordPrecision > 5 {
			opts.CoordPrecision = 5
		}
	}

	b, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	t, err := itinerary.ParseYAML(b)
	if err != nil {
		return err
	}
	if err := itinerary.ResolveDayDates(&t); err != nil {
		return err
	}

	ctx := context.Background()

	if *bundleDir != "" {
		id := strings.TrimSuffix(filepath.Base(*in), filepath.Ext(*in))
		if err := bundle.Build(ctx, t, id, filepath.Dir(*in), *bundleDir, opts); err != nil {
			return fmt.Errorf("bundle: %w", err)
		}
	}

	if *bundleDir != "" && !outputSet {
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
