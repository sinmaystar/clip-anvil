package identity

import "testing"

func TestNormalizeKeyPart(t *testing.T) {
	tests := map[string]string{
		"Shot 03":              "shot_03",
		"机场 出发大厅":              "item",
		"  Product-Luggage!! ": "product_luggage",
		"":                     "item",
	}
	for input, want := range tests {
		if got := NormalizeKeyPart(input); got != want {
			t.Fatalf("NormalizeKeyPart(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSemanticKeyBuilders(t *testing.T) {
	if got := ShotKey(3); got != "shot_03" {
		t.Fatalf("ShotKey = %q", got)
	}
	if got := SceneKey("机场出发大厅", 2); got != "scene_02" {
		t.Fatalf("SceneKey = %q", got)
	}
	if got := KeyElementKey("Product Luggage"); got != "element_product_luggage" {
		t.Fatalf("KeyElementKey = %q", got)
	}
	if got := KeyElementStateKey("element_luggage", "silver reference"); got != "element_luggage.state_silver_reference" {
		t.Fatalf("KeyElementStateKey = %q", got)
	}
	if got := RenderPlanKey("shot_03", "preview_image", 2); got != "shot_03.preview_image.rp2" {
		t.Fatalf("RenderPlanKey = %q", got)
	}
	if got := ArtifactVersionKey("shot_03.preview_image.rp2.output", 1); got != "shot_03.preview_image.rp2.output.v1" {
		t.Fatalf("ArtifactVersionKey = %q", got)
	}
}
