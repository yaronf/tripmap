package tripops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
	"github.com/yaronf/tripmap/internal/store"
)

// Config wires store-backed trip ops.
type Config struct {
	Store         store.Store
	PublicBaseURL string
	RouteMode     string // straight | osrm
	OSRMBaseURL   string // optional; empty uses the public OSRM demo server
}

// Service implements Ops against a Store.
type Service struct {
	store         store.Store
	publicBaseURL string
	routeMode     string
	osrmBase      string
}

// New builds a Service. Store must be non-nil.
func New(cfg Config) *Service {
	return &Service{
		store:         cfg.Store,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
		routeMode:     cfg.RouteMode,
		osrmBase:      strings.TrimRight(cfg.OSRMBaseURL, "/"),
	}
}

var _ Ops = (*Service)(nil)

func (s *Service) SchemaJSON(ctx context.Context) (json.RawMessage, error) {
	return SchemaJSON(ctx)
}

func (s *Service) Summary(ctx context.Context, tripID string) (TripSummary, error) {
	return BuildSummary(ctx, s.store, tripID)
}

func (s *Service) GetYAML(ctx context.Context, tripID, scope string, day int) (YAMLResult, error) {
	return LoadYAML(ctx, s.store, tripID, scope, day)
}

func (s *Service) GetYAMLVersion(ctx context.Context, tripID, versionID string) (YAMLResult, error) {
	return LoadYAMLVersion(ctx, s.store, tripID, versionID)
}

func (s *Service) Patch(ctx context.Context, tripID string, patchJSON []byte) (MutateResult, error) {
	return s.ApplyPatchJSON(ctx, tripID, patchJSON)
}

func (s *Service) ReplaceDayRoutes(ctx context.Context, tripID string, bodyJSON []byte) (MutateResult, error) {
	return s.ApplyReplaceDayRoutesJSON(ctx, tripID, bodyJSON)
}

func (s *Service) ListVersions(ctx context.Context, tripID string, limit int) ([]VersionEntry, error) {
	return ListVersionEntries(ctx, s.store, tripID, limit)
}

// RestoreVersion makes versionID the current YAML (Ops method).
func (s *Service) RestoreVersion(ctx context.Context, tripID, versionID string) (MutateResult, error) {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return MutateResult{}, badRequest(fmt.Errorf("version_id is required"))
	}
	obj, err := s.store.GetYAMLVersion(ctx, tripID, versionID)
	if err != nil {
		return MutateResult{}, notFound(err)
	}
	trip, err := PrepareYAML(obj.Body)
	if err != nil {
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

// Commit writes YAML + meta and regenerates the viewer bundle.
func (s *Service) Commit(ctx context.Context, id string, yamlBytes []byte, meta *store.Meta) (MutateResult, error) {
	vid, err := s.store.PutYAML(ctx, id, yamlBytes)
	if err != nil {
		return MutateResult{}, err
	}
	if err := s.store.PutMeta(ctx, id, *meta); err != nil {
		return MutateResult{}, err
	}

	trip, err := itinerary.ParseYAML(yamlBytes)
	if err != nil {
		return MutateResult{}, err
	}
	_ = itinerary.ResolveDayDates(&trip)
	_ = itinerary.ResolvePlaces(&trip)

	res := MutateResult{
		ID:            id,
		VersionID:     vid,
		SchemaVersion: trip.SchemaVersion,
	}
	if s.publicBaseURL != "" {
		res.ViewerURL = fmt.Sprintf("%s/me/trips/%s/", s.publicBaseURL, id)
	} else {
		res.ViewerURL = fmt.Sprintf("/me/trips/%s/", id)
	}

	if err := s.regenBundle(ctx, id, trip); err != nil {
		res.BundleOK = false
		res.BundleError = err.Error()
	} else {
		res.BundleOK = true
	}
	return res, nil
}

func (s *Service) regenBundle(ctx context.Context, id string, trip itinerary.Trip) error {
	dir, err := os.MkdirTemp("", "tripmap-bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	opts := routebuild.RouteOptions{
		Mode:           s.routeMode,
		SimplifyMeters: 100,
		CoordPrecision: 5,
		Units:          "km",
		OSRMBase:       s.osrmBase,
	}
	if opts.Mode == "" {
		opts.Mode = "osrm"
	}
	if opts.Mode == "osrm" && os.Getenv("TRIPMAP_FORCE_STRAIGHT") == "1" {
		opts.Mode = "straight"
	}
	if err := bundle.Build(ctx, trip, id, "", dir, opts); err != nil {
		if opts.Mode != "straight" {
			opts.Mode = "straight"
			if err2 := bundle.Build(ctx, trip, id, "", dir, opts); err2 != nil {
				return err
			}
		} else {
			return err
		}
	}
	return s.store.UploadBundle(ctx, id, dir)
}
