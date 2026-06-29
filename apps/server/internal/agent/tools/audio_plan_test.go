package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentaudio "github.com/sinmaystar/clip-anvil/internal/agent/audio"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestUpsertAudioPlanNativeToolCreatesNaturalResult(t *testing.T) {
	service := &fakeAudioPlanUpserter{
		output: db.AudioPlan{
			ID:          testUUID(20),
			WorkspaceID: testUUID(1),
			Status:      "waiting_for_user",
			Title:       "营销短视频音频方案",
			CuePlan:     []byte(`[{"shot_ref":"shot_01"}]`),
		},
	}
	tool := NewUpsertAudioPlanNativeTool(service)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: testUUID(1),
		TaskID:      testUUID(3),
	})

	result, err := tool.InvokableRun(ctx, `{
		"brief": "保存全片旁白和 BGM 方案，等待用户确认。",
		"mode": "replace_draft",
		"title": "营销短视频音频方案",
		"language": "zh",
		"voiceover_script": "现在出发，让旅程更轻松。",
		"voice_profile": {"source": "preset", "speaker": "marketing_female_clear"},
		"bgm_plan": {"source": "generated", "provider": "volcengine", "model": "seed-audio-1.0", "style": "轻快电子流行"},
		"cue_plan": [{"shot_ref": "shot_01", "start_sec": 0, "end_sec": 3.2, "text": "现在出发，让旅程更轻松。"}]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "AudioPlan") || !strings.Contains(result, "waiting_for_user") {
		t.Fatalf("result = %s", result)
	}
	if service.input.WorkspaceID != testUUID(1) || service.input.TaskID != testUUID(3) {
		t.Fatalf("runtime not propagated: %#v", service.input)
	}
}

func TestUpsertAudioPlanNativeToolRequiresRuntimeContext(t *testing.T) {
	tool := NewUpsertAudioPlanNativeTool(&fakeAudioPlanUpserter{})

	result, err := tool.InvokableRun(context.Background(), `{"brief":"保存音频方案","mode":"approve"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "缺少 Producer runtime context") {
		t.Fatalf("result = %s", result)
	}
}

func TestUpsertAudioPlanValidationRejectsMissingScript(t *testing.T) {
	tool := NewUpsertAudioPlanNativeTool(&fakeAudioPlanUpserter{})

	result, err := tool.InvokableRun(context.Background(), `{"brief":"保存音频方案","mode":"replace_draft"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "voiceover_script 必填") {
		t.Fatalf("result = %s", result)
	}
}

func TestUpsertAudioPlanValidationRejectsInvalidCueTiming(t *testing.T) {
	tool := NewUpsertAudioPlanNativeTool(&fakeAudioPlanUpserter{})

	result, err := tool.InvokableRun(context.Background(), `{
		"brief":"保存音频方案",
		"mode":"replace_draft",
		"voiceover_script":"一句旁白",
		"cue_plan":[{"shot_ref":"shot_01","start_sec":3,"end_sec":1,"text":"一句旁白"}]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "cue_plan[0] 时间无效") {
		t.Fatalf("result = %s", result)
	}
}

type fakeAudioPlanUpserter struct {
	input  agentaudio.UpsertInput
	output db.AudioPlan
	err    error
}

func (f *fakeAudioPlanUpserter) Upsert(_ context.Context, input agentaudio.UpsertInput) (db.AudioPlan, error) {
	f.input = input
	if f.err != nil {
		return db.AudioPlan{}, f.err
	}
	return f.output, nil
}

func testUUID(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: value}, Valid: true}
}
