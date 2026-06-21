package promptrefs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type Document struct {
	Version int   `json:"version"`
	Refs    []Ref `json:"refs"`
}

type Ref struct {
	NodeID   string `json:"node_id"`
	Label    string `json:"label"`
	NodeType string `json:"node_type"`
}

type RichDocument struct {
	Version int    `json:"version"`
	Source  string `json:"source"`
	Text    string `json:"text"`
}

func Normalize(raw []byte) (Document, []byte, error) {
	trimmed := strings.TrimSpace(string(raw))
	if len(raw) == 0 || trimmed == "" || trimmed == "null" {
		return emptyDocument()
	}
	var doc Document
	if strings.HasPrefix(trimmed, "[") {
		var refs []Ref
		if err := json.Unmarshal(raw, &refs); err != nil {
			return Document{}, nil, err
		}
		return normalizeDocument(Document{Version: 1, Refs: refs})
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, nil, err
	}
	return normalizeDocument(doc)
}

func normalizeDocument(doc Document) (Document, []byte, error) {
	if doc.Version == 0 {
		doc.Version = 1
	}
	if doc.Version != 1 {
		return Document{}, nil, fmt.Errorf("unsupported prompt refs version %d", doc.Version)
	}
	seen := map[string]bool{}
	refs := make([]Ref, 0, len(doc.Refs))
	for _, ref := range doc.Refs {
		ref.NodeID = strings.TrimSpace(ref.NodeID)
		ref.Label = strings.TrimSpace(ref.Label)
		ref.NodeType = strings.TrimSpace(ref.NodeType)
		if ref.NodeID == "" {
			return Document{}, nil, fmt.Errorf("prompt ref missing node_id")
		}
		if _, ok := parseUUID(ref.NodeID); !ok {
			return Document{}, nil, fmt.Errorf("invalid prompt ref node_id")
		}
		if seen[ref.NodeID] {
			continue
		}
		seen[ref.NodeID] = true
		refs = append(refs, ref)
	}
	doc.Refs = refs
	normalized, err := json.Marshal(doc)
	return doc, normalized, err
}

func NormalizeRich(raw []byte, prompt string) ([]byte, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return json.Marshal(RichDocument{Version: 1, Source: "textarea-at", Text: prompt})
	}
	var doc RichDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Version == 0 {
		doc.Version = 1
	}
	if doc.Version != 1 {
		return nil, fmt.Errorf("unsupported prompt rich version %d", doc.Version)
	}
	if strings.TrimSpace(doc.Source) == "" {
		doc.Source = "textarea-at"
	}
	if doc.Text == "" {
		doc.Text = prompt
	}
	return json.Marshal(doc)
}

func emptyDocument() (Document, []byte, error) {
	doc := Document{Version: 1, Refs: []Ref{}}
	normalized, err := json.Marshal(doc)
	return doc, normalized, err
}

func parseUUID(value string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, false
	}
	return id, id.Valid
}
