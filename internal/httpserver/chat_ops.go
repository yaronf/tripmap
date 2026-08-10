package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/viewerchat"
)

// chatTripOps adapts Server store/mutate helpers for viewer chat tools.
type chatTripOps struct {
	s *Server
}

func (o chatTripOps) Summary(ctx context.Context, tripID string) (viewerchat.TripCard, error) {
	obj, err := o.s.store.GetYAML(ctx, tripID)
	if err != nil {
		return viewerchat.TripCard{}, err
	}
	trip, err := itinerary.ParseYAML(obj.Body)
	if err != nil {
		return viewerchat.TripCard{}, err
	}
	titles := make([]string, 0, len(trip.Days))
	for _, d := range trip.Days {
		titles = append(titles, fmt.Sprintf("%d: %s", d.Day, d.Title))
	}
	return viewerchat.TripCard{
		ID:            tripID,
		Title:         trip.Trip,
		Description:   trip.Description,
		Start:         trip.Start,
		Days:          len(trip.Days),
		SchemaVersion: trip.SchemaVersion,
		DayTitles:     titles,
	}, nil
}

func (o chatTripOps) SchemaJSON(context.Context) (json.RawMessage, error) {
	payload := map[string]any{
		"schema_version": itinerary.CurrentSchemaVersion,
		"description":    "tripmap itinerary YAML schema with places catalog",
		"patch_ops":      []string{"swap_days", "update_day", "days", "places", "upsert_stop", "remove_stop", "insert_day", "delete_day"},
		"notes_policy":   "Day and stop notes are human-authored. Do not modify them unless the user explicitly asks. Put enrichment in places.*.info.",
		"update_day_example": map[string]any{
			"update_day": map[string]any{
				"day":   1,
				"title": "Arrive Auckland",
			},
		},
		"add_venue_example": map[string]any{
			"places": map[string]any{
				"pacifica-kaimoana": map[string]any{
					"title":    "Pacifica Kaimoana",
					"lat":      -39.4902,
					"lon":      176.9175,
					"type":     "restaurant",
					"maps_url": "https://www.google.com/maps/search/?api=1&query=Pacifica+Kaimoana+Napier",
				},
			},
			"upsert_stop": map[string]any{
				"day":   12,
				"list":  "stops",
				"place": "pacifica-kaimoana",
				"notes": "Dinner",
			},
		},
	}
	return json.Marshal(payload)
}

func (o chatTripOps) GetYAML(ctx context.Context, tripID string) ([]byte, error) {
	obj, err := o.s.store.GetYAML(ctx, tripID)
	if err != nil {
		return nil, err
	}
	return obj.Body, nil
}

func (o chatTripOps) GetYAMLVersion(ctx context.Context, tripID, versionID string) ([]byte, error) {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return nil, fmt.Errorf("version_id is required")
	}
	obj, err := o.s.store.GetYAMLVersion(ctx, tripID, versionID)
	if err != nil {
		return nil, err
	}
	return obj.Body, nil
}

func (o chatTripOps) GetDay(ctx context.Context, tripID string, day int) (viewerchat.DayDetail, error) {
	body, err := o.GetYAML(ctx, tripID)
	if err != nil {
		return viewerchat.DayDetail{}, err
	}
	return viewerchat.DayDetailFromYAML(body, day)
}

func (o chatTripOps) Patch(ctx context.Context, tripID string, patchJSON []byte) (viewerchat.PatchResult, error) {
	obj, err := o.s.store.GetYAML(ctx, tripID)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	trip, err := itinerary.ParseYAML(obj.Body)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return viewerchat.PatchResult{}, fmt.Errorf("invalid patch json: %w", err)
	}
	if err := itinerary.ApplyPatch(&trip, p); err != nil {
		return viewerchat.PatchResult{}, err
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		return viewerchat.PatchResult{}, err
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		return viewerchat.PatchResult{}, err
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	meta, err := o.s.store.GetMeta(ctx, tripID)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = time.Now().UTC()
	res, _, err := o.s.commitMutate(ctx, tripID, outYAML, &meta)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	return viewerchat.PatchResult{
		ID:        res.ID,
		VersionID: res.VersionID,
		BundleOK:  res.BundleOK,
	}, nil
}

const maxChatVersions = 25

func (o chatTripOps) ListVersions(ctx context.Context, tripID string) ([]viewerchat.VersionEntry, error) {
	vers, err := o.s.store.ListVersions(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if len(vers) > maxChatVersions {
		vers = vers[:maxChatVersions]
	}
	out := make([]viewerchat.VersionEntry, 0, len(vers))
	for _, v := range vers {
		e := viewerchat.VersionEntry{
			VersionID: v.VersionID,
			IsLatest:  v.IsLatest,
		}
		if !v.LastModified.IsZero() {
			e.LastModified = v.LastModified.UTC().Format(time.RFC3339)
		}
		out = append(out, e)
	}
	return out, nil
}

func (o chatTripOps) RestoreVersion(ctx context.Context, tripID, versionID string) (viewerchat.PatchResult, error) {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return viewerchat.PatchResult{}, fmt.Errorf("version_id is required")
	}
	obj, err := o.s.store.GetYAMLVersion(ctx, tripID, versionID)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	trip, err := prepareTripYAML(obj.Body)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	meta, err := o.s.store.GetMeta(ctx, tripID)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = time.Now().UTC()
	res, _, err := o.s.commitMutate(ctx, tripID, outYAML, &meta)
	if err != nil {
		return viewerchat.PatchResult{}, err
	}
	return viewerchat.PatchResult{
		ID:        res.ID,
		VersionID: res.VersionID,
		BundleOK:  res.BundleOK,
	}, nil
}
