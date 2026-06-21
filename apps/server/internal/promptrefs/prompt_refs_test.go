package promptrefs

import "testing"

func TestNormalizeDocumentKeepsFirstDuplicateRef(t *testing.T) {
	raw := []byte(`{"version":1,"refs":[{"node_id":"018f6ef0-5f4f-7e86-9f8c-100000000001","label":"A","node_type":"image"},{"node_id":"018f6ef0-5f4f-7e86-9f8c-100000000001","label":"A again","node_type":"image"}]}`)

	doc, normalized, err := Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Refs) != 1 {
		t.Fatalf("refs len = %d, want 1", len(doc.Refs))
	}
	if got := string(normalized); got != `{"version":1,"refs":[{"node_id":"018f6ef0-5f4f-7e86-9f8c-100000000001","label":"A","node_type":"image"}]}` {
		t.Fatalf("normalized = %s", got)
	}
}

func TestNormalizeDefaultsEmptyDocument(t *testing.T) {
	doc, normalized, err := Normalize(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Refs) != 0 {
		t.Fatalf("refs len = %d, want 0", len(doc.Refs))
	}
	if string(normalized) != `{"version":1,"refs":[]}` {
		t.Fatalf("normalized = %s", normalized)
	}
}

func TestNormalizeAcceptsLegacyArrayDocument(t *testing.T) {
	doc, normalized, err := Normalize([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Refs) != 0 {
		t.Fatalf("refs len = %d, want 0", len(doc.Refs))
	}
	if string(normalized) != `{"version":1,"refs":[]}` {
		t.Fatalf("normalized = %s", normalized)
	}
}

func TestNormalizeRejectsInvalidNodeID(t *testing.T) {
	_, _, err := Normalize([]byte(`{"version":1,"refs":[{"node_id":"bad","label":"A","node_type":"image"}]}`))
	if err == nil {
		t.Fatal("expected invalid node id error")
	}
}
