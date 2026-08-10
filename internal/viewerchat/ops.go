package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/store"
)

// TripCard is a compact trip summary for the system prompt.
type TripCard struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
	Start         string   `json:"start,omitempty"`
	Days          int      `json:"days"`
	SchemaVersion int      `json:"schema_version"`
	DayTitles     []string `json:"day_titles,omitempty"`
}

// DayStop is one route/stop entry on a day for chat tools.
type DayStop struct {
	Place string `json:"place"`
	Title string `json:"title,omitempty"`
	Type  string `json:"type,omitempty"`
	Notes string `json:"notes,omitempty"`
	List  string `json:"list"`
}

// DayDetail is one day's chat-facing projection.
type DayDetail struct {
	Day          int       `json:"day"`
	Title        string    `json:"title"`
	Notes        string    `json:"notes,omitempty"`
	Photo        string    `json:"photo,omitempty"`
	PhotoCaption string    `json:"photo_caption,omitempty"`
	Stops        []DayStop `json:"stops"`
}

// PatchResult is returned after a successful itinerary mutation.
type PatchResult struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id,omitempty"`
	BundleOK  bool   `json:"bundle_ok"`
}

// VersionEntry is one prior YAML revision (S3 object version).
type VersionEntry struct {
	VersionID    string `json:"version_id"`
	LastModified string `json:"last_modified,omitempty"` // RFC3339
	IsLatest     bool   `json:"is_latest,omitempty"`
}

// Preference is one standing user preference exposed to chat tools/prompt.
type Preference struct {
	ID   string   `json:"id"`
	Text string   `json:"text"`
	Tags []string `json:"tags,omitempty"`
}

// TripOps is the in-process trip surface chat tools call (scoped by trip ID).
type TripOps interface {
	Summary(ctx context.Context, tripID string) (TripCard, error)
	SchemaJSON(ctx context.Context) (json.RawMessage, error)
	GetYAML(ctx context.Context, tripID string) ([]byte, error)
	GetYAMLVersion(ctx context.Context, tripID, versionID string) ([]byte, error)
	GetDay(ctx context.Context, tripID string, day int) (DayDetail, error)
	Patch(ctx context.Context, tripID string, patchJSON []byte) (PatchResult, error)
	ListVersions(ctx context.Context, tripID string) ([]VersionEntry, error)
	RestoreVersion(ctx context.Context, tripID, versionID string) (PatchResult, error)
	ListPreferences(ctx context.Context, userSub string) ([]Preference, error)
	SavePreference(ctx context.Context, userSub, id, text string, tags []string) (Preference, error)
	ForgetPreference(ctx context.Context, userSub, id string) error
}

// PreferencesFromDoc maps store doc items for prompt/tools.
func PreferencesFromDoc(doc store.PreferencesDoc) []Preference {
	out := make([]Preference, 0, len(doc.Items))
	for _, it := range doc.Items {
		out = append(out, Preference{ID: it.ID, Text: it.Text, Tags: it.Tags})
	}
	return out
}

// DayDetailFromYAML builds a DayDetail for a 1-based day number.
func DayDetailFromYAML(body []byte, day int) (DayDetail, error) {
	if day < 1 {
		return DayDetail{}, fmt.Errorf("day must be >= 1")
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		return DayDetail{}, err
	}
	var d *itinerary.Day
	for i := range trip.Days {
		if trip.Days[i].Day == day {
			d = &trip.Days[i]
			break
		}
	}
	if d == nil {
		return DayDetail{}, fmt.Errorf("day %d not found", day)
	}
	out := DayDetail{
		Day:          d.Day,
		Title:        d.Title,
		Notes:        d.Notes,
		Photo:        d.Photo,
		PhotoCaption: d.PhotoCaption,
		Stops:        make([]DayStop, 0, len(d.Route)+len(d.Stops)),
	}
	appendStops := func(list []itinerary.Stop, listName string) {
		for _, s := range list {
			title := s.Place
			if p, ok := trip.Places[s.Place]; ok && p.Title != "" {
				title = p.Title
			}
			typ := s.Type
			if typ == "" {
				typ = trip.Places[s.Place].Type
			}
			out.Stops = append(out.Stops, DayStop{
				Place: s.Place,
				Title: title,
				Type:  typ,
				Notes: s.Notes,
				List:  listName,
			})
		}
	}
	appendStops(d.Route, "route")
	appendStops(d.Stops, "stops")
	return out, nil
}
