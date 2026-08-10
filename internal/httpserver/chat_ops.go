package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
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
