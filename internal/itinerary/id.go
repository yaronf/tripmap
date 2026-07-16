package itinerary

import (
	"fmt"
	"regexp"
)

var tripIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidateID checks trip id format.
func ValidateID(id string) error {
	if !tripIDRe.MatchString(id) {
		return fmt.Errorf("invalid trip id %q (want lowercase alphanumeric/hyphen, 1-63 chars)", id)
	}
	return nil
}
