//go:build ignore

// rewrite-yaml-stdout reads itinerary YAML on stdin, normalizes schema/dates,
// and writes YAML on stdout (derived day dates stripped when start is set).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/yaronf/tripmap/internal/itinerary"
)

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}
	trip, err := itinerary.ParseYAML(raw)
	if err != nil {
		fail(err)
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		fail(err)
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		fail(err)
	}
	out, err := itinerary.MarshalYAML(trip)
	if err != nil {
		fail(err)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
