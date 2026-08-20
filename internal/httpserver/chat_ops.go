package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaronf/tripmap/api"
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
		VersionID:     obj.VersionID,
	}, nil
}

func (o chatTripOps) SchemaJSON(context.Context) (json.RawMessage, error) {
	doc, err := api.AgentSchemaDocument()
	if err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

func (o chatTripOps) GetYAML(ctx context.Context, tripID string, scope string, day int) ([]byte, error) {
	obj, err := o.s.store.GetYAML(ctx, tripID)
	if err != nil {
		return nil, err
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "day" {
		if day < 1 {
			return nil, fmt.Errorf("day is required when scope=day")
		}
		return itinerary.BuildDayScopedYAML(obj.Body, day)
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

func (o chatTripOps) Patch(ctx context.Context, tripID string, patchJSON []byte) (viewerchat.MutateResult, error) {
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return viewerchat.MutateResult{}, fmt.Errorf("invalid patch json: %w", err)
	}
	return o.applyPatch(ctx, tripID, p)
}

func (o chatTripOps) ReplaceDayRoutes(ctx context.Context, tripID string, bodyJSON []byte) (viewerchat.MutateResult, error) {
	p, err := itinerary.ParseReplaceDayRoutes(bodyJSON)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	return o.applyPatch(ctx, tripID, p)
}

func (o chatTripOps) applyPatch(ctx context.Context, tripID string, p itinerary.Patch) (viewerchat.MutateResult, error) {
	obj, err := o.s.store.GetYAML(ctx, tripID)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	trip, err := itinerary.ParseYAML(obj.Body)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	if err := itinerary.ApplyPatch(&trip, p); err != nil {
		return viewerchat.MutateResult{}, err
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		return viewerchat.MutateResult{}, err
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		return viewerchat.MutateResult{}, err
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	meta, err := o.s.store.GetMeta(ctx, tripID)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = time.Now().UTC()
	res, _, err := o.s.commitMutate(ctx, tripID, outYAML, &meta)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	return viewerchat.MutateResult{
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

func (o chatTripOps) RestoreVersion(ctx context.Context, tripID, versionID string) (viewerchat.MutateResult, error) {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return viewerchat.MutateResult{}, fmt.Errorf("version_id is required")
	}
	obj, err := o.s.store.GetYAMLVersion(ctx, tripID, versionID)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	trip, err := prepareTripYAML(obj.Body)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	meta, err := o.s.store.GetMeta(ctx, tripID)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = time.Now().UTC()
	res, _, err := o.s.commitMutate(ctx, tripID, outYAML, &meta)
	if err != nil {
		return viewerchat.MutateResult{}, err
	}
	return viewerchat.MutateResult{
		ID:        res.ID,
		VersionID: res.VersionID,
		BundleOK:  res.BundleOK,
	}, nil
}
