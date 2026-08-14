package parser

import "testing"

// Regression: a GP JSON payload without a "metadata" key must still yield a
// non-nil Metadata map, because downstream parser code writes into it without
// a nil check.
func TestDecodeGPTabJSONNoMetadata(t *testing.T) {
	const raw = `{"title":"T","artist":"A","bars":[]}`

	tab, err := decodeGPTabJSON([]byte(raw))
	if err != nil {
		t.Fatalf("decodeGPTabJSON: %v", err)
	}
	if tab.Metadata == nil {
		t.Fatal("Metadata is nil; want non-nil empty map")
	}
	if len(tab.Metadata) != 0 {
		t.Fatalf("Metadata = %v, want empty", tab.Metadata)
	}
}
