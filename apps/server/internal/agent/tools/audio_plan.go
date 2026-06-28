package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	agentaudio "github.com/sinmaystar/clip-anvil/internal/agent/audio"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const toolUpsertAudioPlan = "upsert_audio_plan"

type AudioPlanUpserter interface {
	Upsert(ctx context.Context, input agentaudio.UpsertInput) (db.AudioPlan, error)
}

type UpsertAudioPlanNativeTool struct {
	service AudioPlanUpserter
}

type UpsertAudioPlanInput struct {
	Brief             string                 `json:"brief" jsonschema:"required" jsonschema_description:"一句话说明为什么要写入 AudioPlan。"`
	Mode              string                 `json:"mode" jsonschema:"required,enum=replace_draft,enum=patch,enum=approve,enum=block" jsonschema_description:"replace_draft 创建新的待确认方案；patch 修改当前方案；approve 标记用户已确认；block 标记音频方案阻塞。"`
	Title             string                 `json:"title" jsonschema_description:"音频方案标题。"`
	Language          string                 `json:"language" jsonschema_description:"旁白语言，例如 zh。"`
	TargetDurationSec *float64               `json:"target_duration_sec" jsonschema_description:"目标全片时长，单位秒。"`
	VoiceoverScript   string                 `json:"voiceover_script" jsonschema_description:"全片旁白脚本。replace_draft 和 patch 时必填。"`
	VoiceProfile      map[string]interface{} `json:"voice_profile" jsonschema_description:"音色方向，例如 source、speaker、style。"`
	BGMPlan           map[string]interface{} `json:"bgm_plan" jsonschema_description:"BGM 生成方向。第一版 source 必须是 generated，model 必须是 seed-audio-1.0。"`
	CuePlan           []agentaudio.CueInput  `json:"cue_plan" jsonschema_description:"按 shot_ref 切分的旁白 cue。"`
	GenerationParams  map[string]interface{} `json:"generation_params" jsonschema_description:"后续音频生成参数，例如 format、sample_rate、speech_rate。"`
}

func NewUpsertAudioPlanNativeTool(service AudioPlanUpserter) *UpsertAudioPlanNativeTool {
	return &UpsertAudioPlanNativeTool{service: service}
}

func (t *UpsertAudioPlanNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[UpsertAudioPlanInput](
		toolUpsertAudioPlan,
		"创建、修改或批准当前 workspace 的全片级 AudioPlan。AudioPlan 是 M7 音频链路的事实源，用于保存旁白脚本、音色方向、BGM 生成方向和 shot cue；不会直接生成音频。",
	)
}

func (t *UpsertAudioPlanNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolUpsertAudioPlan, raw, validateUpsertAudioPlanInput)
	if !ok {
		return msg, nil
	}
	if t.service == nil {
		return NaturalToolError(toolUpsertAudioPlan, "audio plan service 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolUpsertAudioPlan)
	if !ok {
		return msg, nil
	}
	plan, err := t.service.Upsert(ctx, agentaudio.UpsertInput{
		WorkspaceID:       runtime.WorkspaceID,
		TaskID:            runtime.TaskID,
		Mode:              strings.TrimSpace(input.Mode),
		Title:             strings.TrimSpace(input.Title),
		Language:          strings.TrimSpace(input.Language),
		TargetDurationSec: ptrFloat64(input.TargetDurationSec),
		VoiceoverScript:   strings.TrimSpace(input.VoiceoverScript),
		VoiceProfile:      input.VoiceProfile,
		BGMPlan:           input.BGMPlan,
		CuePlan:           input.CuePlan,
		GenerationParams:  input.GenerationParams,
	})
	if err != nil {
		return NaturalToolError(toolUpsertAudioPlan, err.Error(), "请读取当前项目上下文，修正 AudioPlan 参数后重试。"), nil
	}
	return NaturalResult{
		Title: "AudioPlan 已更新",
		Items: []NaturalResultItem{
			{Label: "AudioPlan", Value: uuidString(plan.ID)},
			{Label: "状态", Value: plan.Status},
			{Label: "标题", Value: plan.Title},
			{Label: "Cue 数量", Value: fmt.Sprintf("%d", cueCount(plan.CuePlan))},
		},
		Next: audioPlanNextAction(plan.Status),
	}.String(), nil
}

func validateUpsertAudioPlanInput(input UpsertAudioPlanInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.Mode, "replace_draft", "patch", "approve", "block"); err != nil {
		return err
	}
	switch input.Mode {
	case "replace_draft", "patch":
		if err := requireText(input.VoiceoverScript, "voiceover_script"); err != nil {
			return err
		}
		if input.TargetDurationSec != nil && *input.TargetDurationSec <= 0 {
			return fmt.Errorf("target_duration_sec 必须大于 0")
		}
		for index, cue := range input.CuePlan {
			if strings.TrimSpace(cue.ShotRef) == "" {
				return fmt.Errorf("cue_plan[%d].shot_ref 必填", index)
			}
			if strings.TrimSpace(cue.Text) == "" {
				return fmt.Errorf("cue_plan[%d].text 必填", index)
			}
			if cue.StartSec < 0 || cue.EndSec <= cue.StartSec {
				return fmt.Errorf("cue_plan[%d] 时间无效", index)
			}
		}
	}
	return nil
}

func cueCount(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	var cues []any
	if err := json.Unmarshal(raw, &cues); err != nil {
		return 0
	}
	return len(cues)
}

func audioPlanNextAction(status string) string {
	switch status {
	case "waiting_for_user":
		return "等待用户确认旁白、音色和 BGM 方向；用户确认后 Producer 应调用 upsert_audio_plan(mode=approve)。"
	case "approved":
		return "AudioPlan 已确认。M7.2 会基于它派发 voiceover_audio 和 bgm_audio RenderPlan。"
	case "blocked":
		return "AudioPlan 已阻塞。Producer 应向用户说明需要补充或修改的音频决策。"
	default:
		return "继续读取项目上下文，根据 AudioPlan 状态决定下一步。"
	}
}
