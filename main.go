package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
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
	routeMode := fs.String("route", "straight", "route mode: straight or osrm")
	simplify := fs.Float64("simplify", 0, "simplify OSRM route geometry (meters); 0 keeps full detail")
	precision := fs.Int("precision", 0, "decimal places for coordinates (default 6, or 5 with -mymaps)")
	mymaps := fs.Bool("mymaps", false, "optimize for Google My Maps (-simplify 100 -precision 5)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := RouteOptions{Mode: *routeMode, SimplifyMeters: *simplify, CoordPrecision: *precision}
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

	b, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	var t Trip
	if err := yaml.Unmarshal(b, &t); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}

	doc, err := buildDocument(context.Background(), t, opts)
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
