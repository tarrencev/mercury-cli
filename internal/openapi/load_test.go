package openapi

import "testing"

func TestLoadEmbeddedSpecs(t *testing.T) {
	docs, err := LoadEmbeddedSpecs()
	if err != nil {
		t.Fatalf("LoadEmbeddedSpecs error: %v", err)
	}
	if len(docs) < 3 {
		t.Fatalf("expected at least 3 embedded specs, got %d", len(docs))
	}
	for _, d := range docs {
		if d == nil || d.Spec == nil {
			t.Fatalf("nil spec doc")
		}
		if d.Filename == "" || d.Name == "" {
			t.Fatalf("missing spec metadata: %+v", d)
		}
		if len(d.Spec.Paths) == 0 {
			t.Fatalf("spec %s has no paths", d.Filename)
		}
	}
}
