package itinerary

import "fmt"

// ResolvePlaces hydrates Name/Lat/Lon/Type/Notes/Photo/MapsURL/Info on every
// route/stop ref from Trip.Places. Day-local Type/Notes/Photo/PhotoCaption/MapsURL
// override the catalog. Mutates t in place. Safe to call repeatedly.
func ResolvePlaces(t *Trip) error {
	if t == nil {
		return fmt.Errorf("nil trip")
	}
	for di := range t.Days {
		d := &t.Days[di]
		for i := range d.Route {
			if err := resolveStop(t.Places, &d.Route[i]); err != nil {
				return fmt.Errorf("day %d route[%d]: %w", d.Day, i, err)
			}
		}
		for i := range d.Stops {
			if err := resolveStop(t.Places, &d.Stops[i]); err != nil {
				return fmt.Errorf("day %d stops[%d]: %w", d.Day, i, err)
			}
		}
	}
	return nil
}

func resolveStop(places map[string]Place, s *Stop) error {
	if s.Place == "" {
		return fmt.Errorf("missing place id")
	}
	p, ok := places[s.Place]
	if !ok {
		return fmt.Errorf("unknown place %q", s.Place)
	}
	s.Name = p.Title
	s.Lat = p.Lat
	s.Lon = p.Lon
	s.Info = p.Info
	if s.Type == "" {
		s.Type = p.Type
	}
	if s.Notes == "" {
		s.Notes = p.Notes
	}
	if s.Photo == "" {
		s.Photo = p.Photo
	}
	if s.PhotoCaption == "" {
		s.PhotoCaption = p.PhotoCaption
	}
	if s.MapsURL == "" {
		s.MapsURL = p.MapsURL
	}
	return nil
}
