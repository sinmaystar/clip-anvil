package tools

import "testing"

func TestValidateObjectRefRequiresTypeAndKey(t *testing.T) {
	if err := validateObjectRef(ToolObjectRef{Type: "shot", Key: "shot_03"}, "shot_ref"); err != nil {
		t.Fatal(err)
	}
	if err := validateObjectRef(ToolObjectRef{Type: "shot"}, "shot_ref"); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestValidateArtifactRefAllowsCurrentSelector(t *testing.T) {
	ref := ToolArtifactRef{
		Scope:        ToolObjectRef{Type: "shot", Key: "shot_03"},
		ArtifactKind: "preview_image",
		Selector:     "current",
	}
	if err := validateArtifactRef(ref, "target_ref"); err != nil {
		t.Fatal(err)
	}
}
