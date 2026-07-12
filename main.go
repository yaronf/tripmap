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
	in := fs.String("input", "itinerary.yaml", "input YAML itinerary")
	out := fs.String("output", "trip.kml", "output KML file")
	routeMode := fs.String("route", "straight", "route mode: straight or osrm")
	if err := fs.Parse(args); err != nil {
		return err
	}

	b, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	var t Trip
	if err := yaml.Unmarshal(b, &t); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}

	doc, err := buildDocument(context.Background(), t, *routeMode)
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
