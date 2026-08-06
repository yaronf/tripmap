// Command migrate-places converts legacy inline-stop itinerary YAML to a
// places catalog + place refs (schema_version 2).
//
//	go run ./cmd/migrate-places -in itineraries/holland.yaml -out itineraries/holland.yaml
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaronf/tripmap/internal/itinerary"
)

type legacyStop struct {
	Name         string  `yaml:"name"`
	Lat          float64 `yaml:"lat"`
	Lon          float64 `yaml:"lon"`
	Type         string  `yaml:"type,omitempty"`
	Notes        string  `yaml:"notes,omitempty"`
	Photo        string  `yaml:"photo,omitempty"`
	PhotoCaption string  `yaml:"photo_caption,omitempty"`
}

type legacyDay struct {
	Day          int          `yaml:"day"`
	Date         string       `yaml:"date,omitempty"`
	Title        string       `yaml:"title"`
	Route        []legacyStop `yaml:"route,omitempty"`
	Stops        []legacyStop `yaml:"stops,omitempty"`
	Notes        string       `yaml:"notes,omitempty"`
	Photo        string       `yaml:"photo,omitempty"`
	PhotoCaption string       `yaml:"photo_caption,omitempty"`
	Hike         bool         `yaml:"hike,omitempty"`
	Ferry        bool         `yaml:"ferry,omitempty"`
}

type legacyTrip struct {
	SchemaVersion int         `yaml:"schema_version,omitempty"`
	Trip          string      `yaml:"trip"`
	Description   string      `yaml:"description,omitempty"`
	Start         string      `yaml:"start,omitempty"`
	Days          []legacyDay `yaml:"days"`
}

func main() {
	in := flag.String("in", "", "input legacy YAML")
	out := flag.String("out", "", "output places YAML")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: migrate-places -in FILE -out FILE")
		os.Exit(2)
	}
	b, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	var leg legacyTrip
	if err := yaml.Unmarshal(b, &leg); err != nil {
		fatal(err)
	}
	trip, err := convert(leg)
	if err != nil {
		fatal(err)
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		fatal(err)
	}
	if err := itinerary.ValidateBasic(trip); err != nil {
		fatal(err)
	}
	outBytes, err := itinerary.MarshalYAML(trip)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, outBytes, 0644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d places, %d days)\n", *out, len(trip.Places), len(trip.Days))
}

func convert(leg legacyTrip) (itinerary.Trip, error) {
	places := map[string]itinerary.Place{}
	used := map[string]string{} // key -> id
	idOf := func(s legacyStop) string {
		key := fmt.Sprintf("%.6f|%.6f|%s|%s", s.Lat, s.Lon, s.Name, s.Type)
		if id, ok := used[key]; ok {
			return id
		}
		base := slugify(s.Name)
		if base == "" {
			base = "place"
		}
		id := base
		for n := 2; ; n++ {
			if _, exists := places[id]; !exists {
				break
			}
			id = fmt.Sprintf("%s-%d", base, n)
		}
		p := itinerary.Place{
			Title:        s.Name,
			Lat:          s.Lat,
			Lon:          s.Lon,
			Type:         s.Type,
			Notes:        s.Notes,
			Photo:        s.Photo,
			PhotoCaption: s.PhotoCaption,
		}
		places[id] = p
		used[key] = id
		return id
	}
	refOf := func(s legacyStop) itinerary.Stop {
		id := idOf(s)
		ref := itinerary.Stop{Place: id}
		// Only keep day-local type if it differs from catalog (catalog got first-seen type).
		if p := places[id]; s.Type != "" && s.Type != p.Type {
			ref.Type = s.Type
		}
		return ref
	}

	out := itinerary.Trip{
		SchemaVersion: itinerary.CurrentSchemaVersion,
		Trip:          leg.Trip,
		Description:   leg.Description,
		Start:         leg.Start,
		Places:        places,
		Days:          make([]itinerary.Day, 0, len(leg.Days)),
	}
	for _, d := range leg.Days {
		nd := itinerary.Day{
			Day:          d.Day,
			Date:         d.Date,
			Title:        d.Title,
			Notes:        d.Notes,
			Photo:        d.Photo,
			PhotoCaption: d.PhotoCaption,
			Hike:         d.Hike,
			Ferry:        d.Ferry,
		}
		for _, s := range d.Route {
			nd.Route = append(nd.Route, refOf(s))
		}
		for _, s := range d.Stops {
			nd.Stops = append(nd.Stops, refOf(s))
		}
		out.Days = append(out.Days, nd)
	}
	out.Places = places
	return out, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
