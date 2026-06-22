package tools

import (
	"context"
	"testing"

	agentstoryboard "github.com/sinmaystar/clip-anvil/internal/agent/storyboard"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestUpdateStoryboardDefinitionDoesNotWriteCanvas(t *testing.T) {
	tool := NewUpdateStoryboardTool(&fakeStoryboardUpdater{})
	def := tool.Definition()
	if def.Name != "update_storyboard" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Safety.ReadOnly || def.Safety.WritesCanvas || def.Safety.UsesProductionService {
		t.Fatalf("Safety = %#v", def.Safety)
	}
}

func TestUpdateStoryboardExecutesService(t *testing.T) {
	updater := &fakeStoryboardUpdater{out: agentstoryboard.UpdateOutput{
		ShotsCreated: 1,
		Shots:        []db.Shot{{ID: uuidWithByte(2), ClientKey: "shot-01", Title: "开场", Status: "planned"}},
	}}
	tool := NewUpdateStoryboardTool(updater)
	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		Arguments: map[string]any{
			"intent": "replace",
			"shots": []any{map[string]any{
				"client_key": "shot-01",
				"sort_order": 1.0,
				"title":      "开场",
				"brief":      map[string]any{"summary": "开场"},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updater.input.Intent != "replace" || len(updater.input.Shots) != 1 {
		t.Fatalf("input = %#v", updater.input)
	}
	if out.Result["status"] != "succeeded" {
		t.Fatalf("result = %#v", out.Result)
	}
}

func TestUpdateStoryboardAcceptsModelStoryboardShotAliases(t *testing.T) {
	updater := &fakeStoryboardUpdater{out: agentstoryboard.UpdateOutput{
		ShotsCreated: 1,
		Shots:        []db.Shot{{ID: uuidWithByte(2), ClientKey: "shot-01", Title: "分镜 1", Status: "planned"}},
	}}
	tool := NewUpdateStoryboardTool(updater)
	_, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		Arguments: map[string]any{
			"storyboard_shots": []any{map[string]any{
				"shot_number": 1.0,
				"duration":    4.0,
				"content":     "开场特写：博主手持某款好物对准镜头",
				"voice_over":  "家人们！挖到宝了！",
				"ui_overlay":  "商品局部细节浮动弹窗",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updater.input.Shots) != 1 {
		t.Fatalf("shots = %#v", updater.input.Shots)
	}
	shot := updater.input.Shots[0]
	if shot.ClientKey != "shot-01" || shot.SortOrder != 1 || shot.Title != "分镜 1" {
		t.Fatalf("shot = %#v", shot)
	}
	if shot.DurationSec == nil || *shot.DurationSec != 4 {
		t.Fatalf("duration = %#v", shot.DurationSec)
	}
	if shot.Brief["voice_over"] != "家人们！挖到宝了！" || shot.Brief["ui_overlay"] != "商品局部细节浮动弹窗" {
		t.Fatalf("brief = %#v", shot.Brief)
	}
}

func TestUpdateStoryboardRejectsInvalidArguments(t *testing.T) {
	tool := NewUpdateStoryboardTool(&fakeStoryboardUpdater{})
	_, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		Arguments:   map[string]any{"intent": "replace", "shots": "bad"},
	})
	if err == nil {
		t.Fatal("expected invalid arguments error")
	}
}

type fakeStoryboardUpdater struct {
	input agentstoryboard.UpdateInput
	out   agentstoryboard.UpdateOutput
}

func (f *fakeStoryboardUpdater) UpdateStoryboard(_ context.Context, input agentstoryboard.UpdateInput) (agentstoryboard.UpdateOutput, error) {
	f.input = input
	return f.out, nil
}
