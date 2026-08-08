package store

import "testing"

func TestValidateIdempotencyKey(t *testing.T) {
	ok := []string{
		"create-1",
		"smoke-1786129890-put",
		"hello-oauth:abcDEF123_-",
		"abcdef0123456789",
	}
	for _, k := range ok {
		if err := ValidateIdempotencyKey(k); err != nil {
			t.Fatalf("%q: %v", k, err)
		}
	}
	bad := []string{
		"",
		"../trips/holland/itinerary.yaml",
		"../../trips/holland/meta.json",
		"a/b",
		`a\b`,
		"has spaces",
		"bad|pipe",
	}
	for _, k := range bad {
		if err := ValidateIdempotencyKey(k); err == nil {
			t.Fatalf("%q: want error", k)
		}
	}
}

func TestIdemKeyNoEscape(t *testing.T) {
	sk, err := idemKey("ok-key")
	if err != nil || sk != "idempotency/ok-key" {
		t.Fatalf("got %q %v", sk, err)
	}
	if _, err := idemKey("../../trips/holland/itinerary.yaml"); err == nil {
		t.Fatal("expected rejection")
	}
}
