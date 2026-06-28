package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
)

type UpsertStoryboardNativeTool struct {
	service *creative.Service
}

type UpsertStoryboardToolInput struct {
	Brief           string                `json:"brief" jsonschema:"required" jsonschema_description:"本次写入 storyboard 的业务目的，例如创建机场广告第一版场景和分镜。不要超过 160 个中文字符。"`
	Mode            string                `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=replace,enum=archive" jsonschema_description:"create 创建对象；patch 局部更新；replace 替换 scope 下草稿 storyboard；archive 归档对象。patch 可重复提交同一引用或依赖，工程会跳过已存在的关系。"`
	Scope           StoryboardScope       `json:"scope" jsonschema:"required" jsonschema_description:"写入范围。workspace 表示整体 storyboard；scene 表示某个场景；shot 表示某个分镜。"`
	Scenes          []SceneInput          `json:"scenes" jsonschema_description:"要创建或更新的场景列表。"`
	Shots           []ShotInput           `json:"shots" jsonschema_description:"要创建或更新的分镜列表。"`
	ShotKeyElements []ShotKeyElementInput `json:"shot_key_elements" jsonschema_description:"分镜和关键元素状态的引用关系。shot_client_key、element_client_key、state_client_key 必须来自 read_project_context 或本次调用已创建的对象，不要编造。重复关系会被跳过。"`
	Dependencies    []ShotDependencyInput `json:"dependencies" jsonschema_description:"分镜之间的连续性或生产依赖。from/to shot_client_key 必须来自 read_project_context 或本次调用已创建的 shots。重复依赖会被跳过。"`
	Reason          string                `json:"reason" jsonschema_description:"为什么这样写 storyboard，用于审计和后续解释。"`
}

type StoryboardScope struct {
	Type string `json:"type" jsonschema:"required,enum=workspace,enum=scene,enum=shot" jsonschema_description:"写入范围类型。workspace 表示整个项目；scene 表示一个场景；shot 表示一个分镜。"`
	ID   string `json:"id" jsonschema_description:"兼容旧字段：scope 对象内部 ID。workspace 可为空；scene 或 shot 通常使用 key/client_key，不要自行填写内部 ID。"`
}

type SceneInput struct {
	ClientKey   string `json:"client_key" jsonschema:"required" jsonschema_description:"场景稳定业务键，例如 scene_airport_departure_hall。"`
	SortOrder   int32  `json:"sort_order" jsonschema_description:"场景顺序，从 1 开始。"`
	Title       string `json:"title" jsonschema:"required" jsonschema_description:"场景标题，例如机场出发大厅。"`
	Description string `json:"description" jsonschema_description:"场景在故事里的作用和基本内容。"`
	Location    string `json:"location" jsonschema_description:"地点，例如机场出发大厅。"`
	Mood        string `json:"mood" jsonschema_description:"场景情绪和氛围，例如明亮、商务、轻快。"`
}

type ShotInput struct {
	ClientKey        string         `json:"client_key" jsonschema:"required" jsonschema_description:"分镜稳定业务键，例如 shot_01。"`
	SceneClientKey   string         `json:"scene_client_key" jsonschema_description:"所属场景 client_key。"`
	SortOrder        int32          `json:"sort_order" jsonschema_description:"分镜顺序，从 1 开始。"`
	Title            string         `json:"title" jsonschema:"required" jsonschema_description:"分镜标题，例如机场拉箱开场。"`
	ShotKind         string         `json:"shot_kind" jsonschema_description:"分镜类型，例如 lifestyle、product_closeup、transition、cta。"`
	CreativeText     string         `json:"creative_text" jsonschema:"required" jsonschema_description:"创意级画面描述，给用户和 Producer 看。不要写模型 prompt 语法。"`
	NarrativePurpose string         `json:"narrative_purpose" jsonschema_description:"这个分镜在视频结构中的叙事或营销目的。"`
	DurationSec      float64        `json:"duration_sec" jsonschema_description:"分镜目标时长，单位秒。M1 允许为空或 0；若填写应在后续模型能力范围内。"`
	VisualIntent     string         `json:"visual_intent" jsonschema_description:"视觉目标，例如突出银灰色箱体质感、展示机场空间开阔。"`
	ActionText       string         `json:"action_text" jsonschema_description:"主体动作和事件，例如人物单手拉箱，步伐轻快。不要写模型引用语法。"`
	CameraIntent     string         `json:"camera_intent" jsonschema_description:"创意级镜头意图，例如中景跟拍、产品特写。不要写完整 Seedance 三段论。"`
	Dialogue         string         `json:"dialogue" jsonschema_description:"角色台词。没有则留空。"`
	Narration        string         `json:"narration" jsonschema_description:"旁白文案。没有则留空。"`
	AudioPlan        AudioPlanInput `json:"audio_plan" jsonschema_description:"音频计划，包括台词、旁白、音效、BGM。M1 可为空。"`
}

type AudioPlanInput struct {
	Dialogue  string `json:"dialogue" jsonschema_description:"台词说明。"`
	Narration string `json:"narration" jsonschema_description:"旁白说明。"`
	SFX       string `json:"sfx" jsonschema_description:"音效说明，例如机场环境音、行李箱轮子声。"`
	BGM       string `json:"bgm" jsonschema_description:"背景音乐说明。"`
}

type ShotKeyElementInput struct {
	ShotClientKey    string `json:"shot_client_key" jsonschema:"required" jsonschema_description:"分镜 client_key。"`
	ElementClientKey string `json:"element_client_key" jsonschema:"required" jsonschema_description:"关键元素 client_key。"`
	StateClientKey   string `json:"state_client_key" jsonschema_description:"关键元素状态 client_key。为空时使用默认状态。"`
	Role             string `json:"role" jsonschema:"required" jsonschema_description:"该元素在分镜中的创意角色，例如 hero_product、main_character、location、prop、visual_style。"`
	Required         bool   `json:"required" jsonschema_description:"是否为分镜必须出现的元素。"`
	SortOrder        int32  `json:"sort_order" jsonschema_description:"引用排序，越小越重要。"`
}

type ShotDependencyInput struct {
	FromShotClientKey string `json:"from_shot_client_key" jsonschema:"required" jsonschema_description:"依赖来源分镜 client_key。"`
	ToShotClientKey   string `json:"to_shot_client_key" jsonschema:"required" jsonschema_description:"依赖目标分镜 client_key。"`
	DependencyType    string `json:"dependency_type" jsonschema:"required,enum=story_order,enum=last_frame_chain,enum=same_subject_consistency,enum=same_product_consistency,enum=same_scene_consistency,enum=visual_reference,enum=asset_reuse" jsonschema_description:"依赖类型。last_frame_chain 表示目标分镜需要来源分镜尾帧。same_product_consistency 表示商品外观必须一致。"`
	RequiredArtifact  string `json:"required_artifact" jsonschema_description:"依赖需要的产物，例如 last_frame、preview_image、shot_video。M1 可为空。"`
	InjectionRole     string `json:"injection_role" jsonschema_description:"后续 RenderPlan 使用该依赖时的创意级提示，例如 last_frame_chain、same_product_consistency。它不是 Seedance content.role；模型原生角色必须在 RenderPlan reference_bindings.model_role 中填写。"`
	BlockingPhase     string `json:"blocking_phase" jsonschema:"enum=planning,enum=reference_generation,enum=preview_generation,enum=video_generation,enum=review,enum=composition" jsonschema_description:"该依赖阻塞哪个阶段。"`
	Reason            string `json:"reason" jsonschema:"required" jsonschema_description:"为什么需要这个依赖，必须具体说明。"`
}

func NewUpsertStoryboardNativeTool(service *creative.Service) *UpsertStoryboardNativeTool {
	return &UpsertStoryboardNativeTool{service: service}
}

func (t *UpsertStoryboardNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[UpsertStoryboardToolInput](toolUpsertStoryboard, "创建、局部更新、替换或归档 ClipAnvil storyboard 事实，包括 Scene、Shot、分镜引用的关键元素状态，以及分镜之间的连续性依赖。这个工具写的是创意级 storyboard，不写模型原生 prompt。引用关系和依赖必须使用真实存在的 client_key；重复提交同一关系是幂等的。")
}

func (t *UpsertStoryboardNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolUpsertStoryboard, argumentsInJSON, validateUpsertStoryboardInput)
	if !ok {
		return msg, nil
	}
	if msg, ok := serviceOrError(t.service, toolUpsertStoryboard); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolUpsertStoryboard)
	if !ok {
		return msg, nil
	}
	dependencies, skippedSelfDependencies := toCreativeShotDependencies(input.Dependencies)
	out, err := t.service.UpsertStoryboard(ctx, creative.UpsertStoryboardInput{
		WorkspaceID:     runtime.WorkspaceID,
		ThreadID:        runtime.ThreadID,
		TaskID:          runtime.TaskID,
		Brief:           input.Brief,
		Mode:            input.Mode,
		Scope:           creative.StoryboardScope{Type: input.Scope.Type, ID: input.Scope.ID},
		Scenes:          toCreativeScenes(input.Scenes),
		Shots:           toCreativeShots(input.Shots),
		ShotKeyElements: toCreativeShotKeyElements(input.ShotKeyElements),
		Dependencies:    dependencies,
		Reason:          input.Reason,
	})
	if err != nil {
		return naturalErrorFromErr(toolUpsertStoryboard, err), nil
	}
	return NaturalResult{
		Title: "已更新 Storyboard",
		Items: []NaturalResultItem{
			{Label: "场景", Value: fmt.Sprintf("创建 %d 个，更新 %d 个", out.ScenesCreated, out.ScenesUpdated)},
			{Label: "分镜", Value: fmt.Sprintf("创建 %d 个，更新 %d 个", out.ShotsCreated, out.ShotsUpdated)},
			{Label: "引用", Value: fmt.Sprintf("%d 个 shot_key_element", out.ShotKeyElements)},
			{Label: "依赖", Value: fmt.Sprintf("%d 个", out.DependenciesCreated)},
			{Label: "跳过", Value: fmt.Sprintf("%d 个自依赖", skippedSelfDependencies)},
		},
		Next: "读取项目上下文确认 storyboard 和关键元素引用是否完整。",
	}.String(), nil
}

func validateUpsertStoryboardInput(input UpsertStoryboardToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.Mode, "create", "patch", "replace", "archive"); err != nil {
		return err
	}
	if err := requireMode(input.Scope.Type, "workspace", "scene", "shot"); err != nil {
		return err
	}
	for _, shot := range input.Shots {
		if err := requireText(shot.ClientKey, "shots.client_key"); err != nil {
			return err
		}
		if err := requireText(shot.Title, "shots.title"); err != nil {
			return err
		}
		if err := requireText(shot.CreativeText, "shots.creative_text"); err != nil {
			return err
		}
	}
	return nil
}

func toCreativeScenes(input []SceneInput) []creative.SceneInput {
	out := make([]creative.SceneInput, 0, len(input))
	for _, item := range input {
		out = append(out, creative.SceneInput{ClientKey: item.ClientKey, SortOrder: item.SortOrder, Title: item.Title, Description: item.Description, Location: item.Location, Mood: item.Mood})
	}
	return out
}

func toCreativeShots(input []ShotInput) []creative.ShotInput {
	out := make([]creative.ShotInput, 0, len(input))
	for _, item := range input {
		out = append(out, creative.ShotInput{ClientKey: item.ClientKey, SceneClientKey: item.SceneClientKey, SortOrder: item.SortOrder, Title: item.Title, ShotKind: item.ShotKind, CreativeText: item.CreativeText, NarrativePurpose: item.NarrativePurpose, DurationSec: item.DurationSec, VisualIntent: item.VisualIntent, ActionText: item.ActionText, CameraIntent: item.CameraIntent, Dialogue: item.Dialogue, Narration: item.Narration, AudioPlan: creative.AudioPlanInput{Dialogue: item.AudioPlan.Dialogue, Narration: item.AudioPlan.Narration, SFX: item.AudioPlan.SFX, BGM: item.AudioPlan.BGM}})
	}
	return out
}

func toCreativeShotKeyElements(input []ShotKeyElementInput) []creative.ShotKeyElementInput {
	out := make([]creative.ShotKeyElementInput, 0, len(input))
	for _, item := range input {
		out = append(out, creative.ShotKeyElementInput{ShotClientKey: item.ShotClientKey, ElementClientKey: item.ElementClientKey, StateClientKey: item.StateClientKey, Role: item.Role, Required: item.Required, SortOrder: item.SortOrder})
	}
	return out
}

func toCreativeShotDependencies(input []ShotDependencyInput) ([]creative.ShotDependencyInput, int) {
	out := make([]creative.ShotDependencyInput, 0, len(input))
	skippedSelfDependencies := 0
	for _, item := range input {
		if strings.TrimSpace(item.FromShotClientKey) != "" &&
			strings.TrimSpace(item.FromShotClientKey) == strings.TrimSpace(item.ToShotClientKey) {
			skippedSelfDependencies++
			continue
		}
		out = append(out, creative.ShotDependencyInput{FromShotClientKey: item.FromShotClientKey, ToShotClientKey: item.ToShotClientKey, DependencyType: item.DependencyType, RequiredArtifact: item.RequiredArtifact, InjectionRole: item.InjectionRole, BlockingPhase: item.BlockingPhase, Reason: item.Reason})
	}
	return out, skippedSelfDependencies
}
