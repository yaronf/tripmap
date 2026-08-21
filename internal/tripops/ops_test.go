package tripops

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/yaronf/tripmap/internal/store"
)

func TestHTTPStatus(t *testing.T) {
	if HTTPStatus(nil) != http.StatusOK {
		t.Fatal("nil")
	}
	if HTTPStatus(notFound(errors.New("x"))) != http.StatusNotFound {
		t.Fatal("not found")
	}
	if HTTPStatus(badRequest(errors.New("x"))) != http.StatusBadRequest {
		t.Fatal("bad request")
	}
	if HTTPStatus(errors.New("other")) != http.StatusInternalServerError {
		t.Fatal("other")
	}
}

func TestLoadYAMLDayRequiresDay(t *testing.T) {
	st := store.NewMem()
	_, err := LoadYAML(context.Background(), st, "nope", "day", 0)
	if !errors.Is(err, errNotFound) && HTTPStatus(err) != http.StatusNotFound {
		// missing trip → not found before day check
		t.Fatalf("expected not found for missing trip, got %v", err)
	}
}
