package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaronf/tripmap/api"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/store"
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
	doc, err := api.AgentSchemaDocument()
	if err != nil {
		return nil, err
	}
	return json.Marshal(doc)
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

func (o chatTripOps) CommitYAML(ctx context.Context, tripID string, yamlBody []byte) (viewerchat.PatchResult, error) {
	trip, err := prepareTripYAML(yamlBody)
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

func (o chatTripOps) ListPreferences(ctx context.Context, userSub string) ([]viewerchat.Preference, error) {
	doc, err := o.s.store.GetPreferences(ctx, userSub)
	if err != nil {
		return nil, err
	}
	return viewerchat.PreferencesFromDoc(doc), nil
}

func (o chatTripOps) SavePreference(ctx context.Context, userSub, id, text string, tags []string) (viewerchat.Preference, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return viewerchat.Preference{}, fmt.Errorf("text is required")
	}
	if len([]rune(text)) > store.MaxPreferenceText {
		return viewerchat.Preference{}, fmt.Errorf("text exceeds %d characters", store.MaxPreferenceText)
	}
	doc, err := o.s.store.GetPreferences(ctx, userSub)
	if err != nil {
		return viewerchat.Preference{}, err
	}
	now := time.Now().UTC()
	id = strings.TrimSpace(id)
	if id == "" {
		id, err = newPreferenceID()
		if err != nil {
			return viewerchat.Preference{}, err
		}
	}
	cleanTags := normalizePrefTags(tags)
	updated := false
	for i := range doc.Items {
		if doc.Items[i].ID == id {
			doc.Items[i].Text = text
			doc.Items[i].Tags = cleanTags
			doc.Items[i].UpdatedAt = now
			updated = true
			break
		}
	}
	if !updated {
		if len(doc.Items) >= store.MaxPreferenceItems {
			return viewerchat.Preference{}, fmt.Errorf("at most %d preferences", store.MaxPreferenceItems)
		}
		doc.Items = append(doc.Items, store.PreferenceItem{
			ID:        id,
			Text:      text,
			Tags:      cleanTags,
			UpdatedAt: now,
		})
	}
	doc.UpdatedAt = now
	if err := o.s.store.PutPreferences(ctx, userSub, doc); err != nil {
		return viewerchat.Preference{}, err
	}
	return viewerchat.Preference{ID: id, Text: text, Tags: cleanTags}, nil
}

func (o chatTripOps) ForgetPreference(ctx context.Context, userSub, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("id is required")
	}
	doc, err := o.s.store.GetPreferences(ctx, userSub)
	if err != nil {
		return err
	}
	out := make([]store.PreferenceItem, 0, len(doc.Items))
	found := false
	for _, it := range doc.Items {
		if it.ID == id {
			found = true
			continue
		}
		out = append(out, it)
	}
	if !found {
		return fmt.Errorf("preference %q not found", id)
	}
	doc.Items = out
	doc.UpdatedAt = time.Now().UTC()
	return o.s.store.PutPreferences(ctx, userSub, doc)
}

func (o chatTripOps) ListLearnings(ctx context.Context, userSub string) ([]viewerchat.Learning, error) {
	doc, err := o.s.store.GetLearnings(ctx, userSub)
	if err != nil {
		return nil, err
	}
	return viewerchat.LearningsFromDoc(doc), nil
}

func (o chatTripOps) SaveLearning(ctx context.Context, userSub, id, text string, tags []string) (viewerchat.Learning, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return viewerchat.Learning{}, fmt.Errorf("text is required")
	}
	if len([]rune(text)) > store.MaxLearningText {
		return viewerchat.Learning{}, fmt.Errorf("text exceeds %d characters", store.MaxLearningText)
	}
	doc, err := o.s.store.GetLearnings(ctx, userSub)
	if err != nil {
		return viewerchat.Learning{}, err
	}
	now := time.Now().UTC()
	id = strings.TrimSpace(id)
	if id == "" {
		id, err = newLearningID()
		if err != nil {
			return viewerchat.Learning{}, err
		}
	}
	cleanTags := normalizePrefTags(tags)
	updated := false
	for i := range doc.Items {
		if doc.Items[i].ID == id {
			doc.Items[i].Text = text
			doc.Items[i].Tags = cleanTags
			doc.Items[i].UpdatedAt = now
			updated = true
			break
		}
	}
	if !updated {
		if len(doc.Items) >= store.MaxLearningItems {
			return viewerchat.Learning{}, fmt.Errorf("at most %d learnings", store.MaxLearningItems)
		}
		doc.Items = append(doc.Items, store.LearningItem{
			ID:        id,
			Text:      text,
			Tags:      cleanTags,
			UpdatedAt: now,
		})
	}
	doc.UpdatedAt = now
	if err := o.s.store.PutLearnings(ctx, userSub, doc); err != nil {
		return viewerchat.Learning{}, err
	}
	return viewerchat.Learning{ID: id, Text: text, Tags: cleanTags}, nil
}

func (o chatTripOps) ForgetLearning(ctx context.Context, userSub, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("id is required")
	}
	doc, err := o.s.store.GetLearnings(ctx, userSub)
	if err != nil {
		return err
	}
	out := make([]store.LearningItem, 0, len(doc.Items))
	found := false
	for _, it := range doc.Items {
		if it.ID == id {
			found = true
			continue
		}
		out = append(out, it)
	}
	if !found {
		return fmt.Errorf("learning %q not found", id)
	}
	doc.Items = out
	doc.UpdatedAt = time.Now().UTC()
	return o.s.store.PutLearnings(ctx, userSub, doc)
}

func newPreferenceID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "pref_" + hex.EncodeToString(b[:]), nil
}

func newLearningID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "learn_" + hex.EncodeToString(b[:]), nil
}

func normalizePrefTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) >= 8 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
