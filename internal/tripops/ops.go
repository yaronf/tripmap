// Package tripops is the shared itinerary agent surface used by HTTP/MCP and
// in-viewer chat. Business logic lives here; transports only bind auth and I/O.
package tripops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yaronf/tripmap/api"
	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/store"
)

// Ops is the trip-scoped agent surface (no listTrips / createTrip).
type Ops interface {
	SchemaJSON(ctx context.Context) (json.RawMessage, error)
	Summary(ctx context.Context, tripID string) (TripSummary, error)
	GetYAML(ctx context.Context, tripID, scope string, day int) (YAMLResult, error)
	GetYAMLVersion(ctx context.Context, tripID, versionID string) (YAMLResult, error)
	Patch(ctx context.Context, tripID string, patchJSON []byte) (MutateResult, error)
	ReplaceDayRoutes(ctx context.Context, tripID string, bodyJSON []byte) (MutateResult, error)
	ListVersions(ctx context.Context, tripID string, limit int) ([]VersionEntry, error)
	RestoreVersion(ctx context.Context, tripID, versionID string) (MutateResult, error)
	EstimateDrive(ctx context.Context, tripID string, points []DriveWaypoint) (DriveEstimate, error)
}

// DayDriveStats is precomputed OSRM drive totals for one day (from bundle trip.json).
type DayDriveStats struct {
	Day       int     `json:"day"`
	DriveDist float64 `json:"drive_dist,omitempty"`
	DriveMin  int     `json:"drive_min,omitempty"`
}

// TripSummary matches OpenAPI TripSummary (plus optional agent fields).
type TripSummary struct {
	ID            string          `json:"id"`
	VersionID     string          `json:"version_id,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Trip          string          `json:"trip"`
	Description   string          `json:"description,omitempty"`
	Start         string          `json:"start,omitempty"`
	Days          int             `json:"days"`
	UpdatedAt     time.Time       `json:"updated_at,omitempty"`
	DayTitles     []string        `json:"day_titles,omitempty"`
	Units         string          `json:"units,omitempty"` // km | mi — bundle distance unit for drive_dist
	DayStats      []DayDriveStats `json:"day_stats,omitempty"`
}

// YAMLResult is itinerary YAML plus version id.
type YAMLResult struct {
	Body      []byte
	VersionID string
}

// MutateResult matches OpenAPI MutateResult.
type MutateResult struct {
	ID            string `json:"id"`
	VersionID     string `json:"version_id,omitempty"`
	SchemaVersion int    `json:"schema_version"`
	ViewerURL     string `json:"viewer_url,omitempty"`
	BundleOK      bool   `json:"bundle_ok"`
	BundleError   string `json:"bundle_error,omitempty"`
}

// VersionEntry is one YAML revision for agent tools.
type VersionEntry struct {
	VersionID    string `json:"version_id"`
	LastModified string `json:"last_modified,omitempty"` // RFC3339
	IsLatest     bool   `json:"is_latest,omitempty"`
}

// OpenAPI operation summaries — single source for chat tool one-liners.
const (
	SummaryGetSchema        = "Itinerary schema and version"
	SummaryGetTrip          = "Compact trip card with day titles and per-day drive_dist/drive_min from bundle (use for drive-time questions; no YAML needed)"
	SummaryGetTripYAML      = "Get itinerary YAML"
	SummaryPatchTrip        = "Patch places info, day narrative, or structure"
	SummaryReplaceDayRoutes = "Replace full day routes (overnight / endpoint). Args: days object keyed by day number (not array); include N and N+1 for overnight moves; optional places."
	SummaryListVersions     = "List YAML versions"
	SummaryGetVersion       = "Get YAML for a prior version"
	SummaryRestoreVersion   = "Restore a prior YAML version"
)

var (
	errNotFound   = errors.New("not found")
	errBadRequest = errors.New("bad request")
)

// HTTPStatus maps tripops errors to HTTP codes for agent handlers.
func HTTPStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, errNotFound):
		return http.StatusNotFound
	case errors.Is(err, errBadRequest):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func notFound(err error) error {
	if err == nil {
		return errNotFound
	}
	return fmt.Errorf("%w: %v", errNotFound, err)
}

func badRequest(err error) error {
	if err == nil {
		return errBadRequest
	}
	return fmt.Errorf("%w: %v", errBadRequest, err)
}

// SchemaJSON returns the agent schema document.
func SchemaJSON(context.Context) (json.RawMessage, error) {
	doc, err := api.AgentSchemaDocument()
	if err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

// BuildSummary loads YAML and builds a TripSummary.
func BuildSummary(ctx context.Context, st store.Store, tripID string) (TripSummary, error) {
	obj, err := st.GetYAML(ctx, tripID)
	if err != nil {
		return TripSummary{}, notFound(err)
	}
	trip, err := itinerary.ParseYAML(obj.Body)
	if err != nil {
		return TripSummary{}, err
	}
	titles := make([]string, 0, len(trip.Days))
	for _, d := range trip.Days {
		titles = append(titles, fmt.Sprintf("%d: %s", d.Day, d.Title))
	}
	sum := TripSummary{
		ID:            tripID,
		VersionID:     obj.VersionID,
		SchemaVersion: trip.SchemaVersion,
		Trip:          trip.Trip,
		Description:   trip.Description,
		Start:         trip.Start,
		Days:          len(trip.Days),
		DayTitles:     titles,
	}
	if meta, err := st.GetMeta(ctx, tripID); err == nil {
		sum.UpdatedAt = meta.UpdatedAt
	}
	mergeBundleDriveStats(ctx, st, tripID, &sum)
	return sum, nil
}

func mergeBundleDriveStats(ctx context.Context, st store.Store, tripID string, sum *TripSummary) {
	if sum == nil {
		return
	}
	body, _, err := st.GetBundleObject(ctx, tripID, "trip.json")
	if err != nil {
		return
	}
	var tj bundle.TripJSON
	if err := json.Unmarshal(body, &tj); err != nil {
		return
	}
	sum.Units = tj.Units
	if len(tj.Days) == 0 {
		return
	}
	stats := make([]DayDriveStats, 0, len(tj.Days))
	for _, d := range tj.Days {
		stats = append(stats, DayDriveStats{
			Day:       d.Day,
			DriveDist: d.DriveDist,
			DriveMin:  d.DriveMin,
		})
	}
	sum.DayStats = stats
}

// LoadYAML returns full or day-scoped YAML.
// Empty scope means full (HTTP/MCP default). Callers that want day-default
// (viewer chat) must pass scope=day explicitly.
func LoadYAML(ctx context.Context, st store.Store, tripID, scope string, day int) (YAMLResult, error) {
	obj, err := st.GetYAML(ctx, tripID)
	if err != nil {
		return YAMLResult{}, notFound(err)
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	body := obj.Body
	if scope == "day" {
		if day < 1 {
			return YAMLResult{}, badRequest(fmt.Errorf("day is required when scope=day"))
		}
		scoped, err := itinerary.BuildDayScopedYAML(body, day)
		if err != nil {
			return YAMLResult{}, badRequest(err)
		}
		body = scoped
	}
	return YAMLResult{Body: body, VersionID: obj.VersionID}, nil
}

// LoadYAMLVersion returns a prior revision's YAML.
func LoadYAMLVersion(ctx context.Context, st store.Store, tripID, versionID string) (YAMLResult, error) {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return YAMLResult{}, badRequest(fmt.Errorf("version_id is required"))
	}
	obj, err := st.GetYAMLVersion(ctx, tripID, versionID)
	if err != nil {
		return YAMLResult{}, notFound(err)
	}
	return YAMLResult{Body: obj.Body, VersionID: obj.VersionID}, nil
}

// ListVersionEntries lists versions; limit <= 0 means no cap.
func ListVersionEntries(ctx context.Context, st store.Store, tripID string, limit int) ([]VersionEntry, error) {
	vers, err := st.ListVersions(ctx, tripID)
	if err != nil {
		return nil, notFound(err)
	}
	if limit > 0 && len(vers) > limit {
		vers = vers[:limit]
	}
	out := make([]VersionEntry, 0, len(vers))
	for _, v := range vers {
		e := VersionEntry{
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

// ApplyPatchJSON unmarshals a TripPatch and applies it.
func (s *Service) ApplyPatchJSON(ctx context.Context, tripID string, patchJSON []byte) (MutateResult, error) {
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return MutateResult{}, badRequest(fmt.Errorf("invalid patch json: %w", err))
	}
	return s.ApplyPatch(ctx, tripID, p)
}

// ApplyReplaceDayRoutesJSON parses replaceDayRoutes body and applies it.
func (s *Service) ApplyReplaceDayRoutesJSON(ctx context.Context, tripID string, bodyJSON []byte) (MutateResult, error) {
	p, err := itinerary.ParseReplaceDayRoutes(bodyJSON)
	if err != nil {
		return MutateResult{}, badRequest(err)
	}
	return s.ApplyPatch(ctx, tripID, p)
}

// ApplyPatch loads, patches, validates, and commits an itinerary.
func (s *Service) ApplyPatch(ctx context.Context, tripID string, p itinerary.Patch) (MutateResult, error) {
	obj, err := s.store.GetYAML(ctx, tripID)
	if err != nil {
		return MutateResult{}, notFound(err)
	}
	trip, err := itinerary.ParseYAML(obj.Body)
	if err != nil {
		return MutateResult{}, err
	}
	if err := itinerary.ApplyPatch(&trip, p); err != nil {
		return MutateResult{}, badRequest(err)
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		return MutateResult{}, badRequest(err)
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		return MutateResult{}, badRequest(err)
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		return MutateResult{}, err
	}
	meta, err := s.store.GetMeta(ctx, tripID)
	if err != nil {
		return MutateResult{}, err
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = metaNow()
	return s.Commit(ctx, tripID, outYAML, &meta)
}

func metaNow() time.Time {
	return time.Now().UTC()
}

// PrepareYAML parses and validates itinerary YAML for create/put/restore.
func PrepareYAML(b []byte) (itinerary.Trip, error) {
	trip, err := itinerary.ParseYAML(b)
	if err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.ValidateBasic(trip); err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.ResolvePlaces(&trip); err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		return itinerary.Trip{}, err
	}
	return trip, nil
}
