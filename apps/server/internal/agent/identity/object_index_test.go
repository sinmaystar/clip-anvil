package identity

import (
	"strings"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestRenderObjectIndexIncludesActionableKeys(t *testing.T) {
	rows := []db.AgentObjectIndex{
		{ObjectType: ObjectScene, SemanticKey: "scene_airport", DisplayName: "机场出发大厅", Status: "planned", Kind: "scene", SortOrder: 1},
		{ObjectType: ObjectShot, SemanticKey: "shot_01", DisplayName: "产品开场", ParentSemanticKey: "scene_airport", Status: "preview_ready", Kind: "lifestyle", SortOrder: 1},
		{ObjectType: ObjectRenderPlan, SemanticKey: "shot_01.preview_image.rp1", DisplayName: "预览图计划", ParentSemanticKey: "shot_01", Status: "succeeded", Kind: "preview_image", SortOrder: 1},
		{ObjectType: ObjectArtifactVersion, SemanticKey: "shot_01.preview_image.rp1.output.v1", DisplayName: "预览图 v1", ParentSemanticKey: "shot_01.preview_image.rp1.output", Status: "succeeded", Kind: "preview_image", SortOrder: 1},
	}
	text := RenderObjectIndex(rows)
	for _, want := range []string{
		"Scene scene_airport｜机场出发大厅｜planned",
		"Shot shot_01｜产品开场｜preview_ready",
		"RenderPlan shot_01.preview_image.rp1｜preview_image｜succeeded",
		"Artifact shot_01.preview_image.rp1.output.v1｜preview_image｜succeeded",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("index text missing %q:\n%s", want, text)
		}
	}
}
