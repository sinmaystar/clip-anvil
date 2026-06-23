package uimessage

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeSchemaAndBlocks(t *testing.T) {
	envelope := Envelope{
		Schema: SchemaV1,
		Blocks: []Block{
			MarkdownBlock{BaseBlock: BaseBlock{ID: "blk_text"}, Text: "hello"},
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded["schema"] != "clipanvil.agent.message.v1" {
		t.Fatalf("schema = %#v", decoded["schema"])
	}
	blocks, ok := decoded["blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("blocks = %#v", decoded["blocks"])
	}
	first := blocks[0].(map[string]any)
	if first["type"] != "markdown" || first["text"] != "hello" {
		t.Fatalf("first block = %#v", first)
	}
}

func TestExtractMarkdownTextSkipsThinkingAndToolStatus(t *testing.T) {
	raw := []byte(`{
	  "schema":"clipanvil.agent.message.v1",
	  "blocks":[
	    {"id":"blk_thinking","type":"thinking","text":"hidden reasoning","status":"done","default_collapsed":true},
	    {"id":"blk_answer","type":"markdown","text":"visible answer"},
	    {"id":"blk_tool","type":"tool_status","tool_call_id":"call_1","tool_name":"read_workspace_context","label":"done","status":"succeeded"}
	  ]
	}`)
	texts := ExtractMarkdownTexts(raw)
	if len(texts) != 1 || texts[0] != "visible answer" {
		t.Fatalf("texts = %#v, want visible answer only", texts)
	}
}

func TestExtractAttachmentsFromBlocks(t *testing.T) {
	raw := []byte(`{
	  "schema":"clipanvil.agent.message.v1",
	  "blocks":[
	    {"id":"blk_text","type":"markdown","text":"see image"},
	    {"id":"blk_attachment","type":"attachment","attachments":[
	      {"asset_id":"asset-1","node_id":"node-1","kind":"image","name":"hero.png","mime":"image/png","size_bytes":123}
	    ]}
	  ]
	}`)
	attachments := ExtractAttachments(raw)
	if len(attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(attachments))
	}
	if attachments[0].AssetID != "asset-1" || attachments[0].Kind != "image" {
		t.Fatalf("attachment = %#v", attachments[0])
	}
}

func TestFinalVideoCardBlockMarshalsProtocolShape(t *testing.T) {
	envelope := Envelope{
		Schema: SchemaV1,
		Blocks: []Block{
			FinalVideoCardBlock{
				BaseBlock:   BaseBlock{ID: "blk_final"},
				Status:      "ready",
				NodeID:      "node-1",
				VersionID:   "version-1",
				AssetID:     "asset-1",
				Title:       "成片",
				URL:         "http://localhost/final.mp4",
				SourceShots: []string{"shot-01", "shot-02"},
			},
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded struct {
		Blocks []struct {
			Type        string   `json:"type"`
			Status      string   `json:"status"`
			NodeID      string   `json:"node_id"`
			VersionID   string   `json:"version_id"`
			AssetID     string   `json:"asset_id"`
			SourceShots []string `json:"source_shots"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(decoded.Blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(decoded.Blocks))
	}
	block := decoded.Blocks[0]
	if block.Type != "final_video_card" || block.Status != "ready" {
		t.Fatalf("block = %#v", block)
	}
	if block.NodeID != "node-1" || block.VersionID != "version-1" || block.AssetID != "asset-1" {
		t.Fatalf("ids = %#v", block)
	}
	if len(block.SourceShots) != 2 || block.SourceShots[1] != "shot-02" {
		t.Fatalf("source_shots = %#v", block.SourceShots)
	}
}
