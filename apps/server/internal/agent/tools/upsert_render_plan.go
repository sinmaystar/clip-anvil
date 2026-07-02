package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/renderplan"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type UpsertRenderPlanNativeTool struct {
	service        *renderplan.Service
	submitter      *RenderPlanSubmitter
	referenceStore RenderPlanReferenceStore
}

type RenderPlanReferenceStore interface {
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
}

type UpsertRenderPlanToolInput struct {
	Brief                string                    `json:"brief" jsonschema:"required" jsonschema_description:"本次写入 RenderPlan 的业务目的，例如为机场出发大厅状态创建 Seedream reference image 计划。不要超过 160 个中文字符。"`
	Mode                 string                    `json:"mode" jsonschema:"required,enum=create,enum=update_draft,enum=fork_from,enum=mark_blocked" jsonschema_description:"create 创建新计划；update_draft 修改未执行草稿；fork_from 基于旧计划创建新 revision；mark_blocked 记录无法继续的阻塞原因。"`
	GenerationText       string                    `json:"generation_text" jsonschema:"required" jsonschema_description:"本计划最重要的自由文本生成说明。请用自然语言完整写清楚要生成什么，不要写 JSON 字符串。必须覆盖：主体是谁以及必须保持的稳定特征；场景/空间/时间；光线和色彩；镜头语言、景别、构图和运镜；主体动作或视频事件顺序；整体风格和商业质感；音频、旁白或字幕诉求；必须避免的错误和不一致。图片计划写成一段详细画面描述；视频计划写成一段可执行镜头描述，只保留一个主要动作和一个主要运镜。建议 300 到 900 个中文字符，宁可写在这里，也不要把 prompt_parts、audit_hints、subject_bindings 填成很长的多层 JSON。"`
	RenderPlanID         string                    `json:"render_plan_id" jsonschema_description:"兼容旧字段：update_draft 或 mark_blocked 时的目标 RenderPlan 内部 ID。模型通常不要填写；create 时为空。"`
	ForkFromRenderPlanID string                    `json:"fork_from_render_plan_id" jsonschema_description:"兼容旧字段：mode=fork_from 时的来源 RenderPlan 内部 ID。通常由当前任务上下文提供，不能和 render_plan_id 同时作为写入目标。"`
	Scope                RenderPlanScopeInput      `json:"scope" jsonschema_description:"RenderPlan 归属对象。通常可以省略，工具会继承当前 Craftsman task scope；只有确有必要时才填写，且必须与当前 task scope 一致。"`
	TargetPhase          string                    `json:"target_phase" jsonschema:"enum=reference_image,enum=preview_image,enum=shot_video,enum=voiceover_audio,enum=bgm_audio" jsonschema_description:"目标阶段。通常可以省略，工具会继承当前 Craftsman task target_phase。reference_image 为关键元素参考图；preview_image 为分镜预览图；shot_video 为分镜视频；voiceover_audio 和 bgm_audio 为全片 AudioPlan 音频计划。"`
	TaskType             string                    `json:"task_type" jsonschema:"enum=generate,enum=edit,enum=extend,enum=bridge" jsonschema_description:"生成任务类型。通常省略，默认 generate；只有明确做编辑、延长、桥接时填写 edit/extend/bridge。"`
	ModelPromptProfile   string                    `json:"model_prompt_profile" jsonschema:"enum=seedream_5_image,enum=seedance_2_video,enum=seed_audio_1,enum=motion_shot_video" jsonschema_description:"通常省略，由 target_phase 自动推导：reference_image/preview_image 使用 seedream_5_image，shot_video 默认使用 seedance_2_video；若该分镜是商品图、卖点、对比或 CTA 的低成本图片动效视频，可显式填写 motion_shot_video；voiceover_audio/bgm_audio 使用 seed_audio_1。"`
	Operation            string                    `json:"operation" jsonschema:"enum=text_to_image,enum=image_to_image,enum=multi_image_to_image,enum=text_to_video,enum=image_to_video_first_frame,enum=image_to_video_first_last_frame,enum=multi_modal_reference_video,enum=video_edit,enum=video_extend,enum=video_bridge,enum=text_to_audio,enum=image_to_motion_video" jsonschema_description:"通常省略，由 target_phase 和 reference_bindings 自动推导。Seedance 使用 text_to_video / image_to_video_first_frame 等视频操作；motion_shot_video 使用 image_to_motion_video。只有你明确知道需要 edit/extend/bridge 或特殊首尾帧时才填写。不要填 provider API 私有枚举。"`
	ReferenceBindings    []ReferenceBindingInput   `json:"reference_bindings" jsonschema_description:"本计划使用的参考资源绑定。只填写真实需要的资源，通常 0 到 3 个；不要为了完整而编造引用。"`
	SubjectBindings      []SubjectBindingInput     `json:"subject_bindings" jsonschema_description:"可选高级字段。只有需要显式声明主体句柄时才填写；一般把主体与一致性要求写进 generation_text 即可，避免长 JSON。"`
	PromptParts          RenderPromptPartsInput    `json:"prompt_parts" jsonschema_description:"可选高级字段。一般留空，工具会把 generation_text 编译为 prompt_parts.objective；视频计划也会用 generation_text 兜底 action。只有确实需要结构化拆分时，才填写少量字段。"`
	Params               RenderPlanParamsInput     `json:"params" jsonschema_description:"模型参数草案，例如比例、时长、分辨率、是否返回尾帧。工具和 PromptCompiler 会校验。"`
	AuditHints           RenderPlanAuditHintsInput `json:"audit_hints" jsonschema_description:"可选高级字段。一般留空；风险、疑问和修复建议优先用 generation_text 或 blocker.message 简短表达。"`
	Blocker              RenderPlanBlockerInput    `json:"blocker" jsonschema_description:"mode=mark_blocked 时填写，说明为什么不能继续生成。"`
	Rationale            string                    `json:"rationale" jsonschema_description:"为什么这样组织 prompt parts、参考资源和参数。保持简短，面向 Producer 可读；为空时工具会用 brief 兜底。"`
}

type RenderPlanScopeInput struct {
	Type string `json:"type" jsonschema:"enum=key_element_state,enum=shot,enum=audio_plan" jsonschema_description:"RenderPlan 归属类型。通常省略并继承当前 Craftsman task。key_element_state 通常对应 reference_image；shot 对应 preview_image 或 shot_video；audio_plan 对应 voiceover_audio 或 bgm_audio。"`
	ID   string `json:"id" jsonschema_description:"兼容旧字段：归属对象内部 ID。通常省略并继承当前 Craftsman task。"`
	Key  string `json:"key" jsonschema_description:"归属对象语义键，例如 scene_main.shot_01 或 element_airport.state_morning。通常省略并继承当前 Craftsman task；模型不要编造。"`
}

type ReferenceBindingInput struct {
	ClientKey      string `json:"client_key" jsonschema:"required" jsonschema_description:"稳定业务键，例如 ref_product_luggage_default、ref_airport_scene_morning。用于重试和审计。"`
	SourceType     string `json:"source_type" jsonschema:"required,enum=key_element_state,enum=media_node,enum=artifact_version,enum=shot_output" jsonschema_description:"参考来源类型。优先使用 key_element_state 或 artifact_version，而不是裸素材。"`
	SourceID       string `json:"source_id" jsonschema:"required" jsonschema_description:"参考来源标识。media_node 可填写 read_project_context 返回的 semantic_key 或当前 workspace 内唯一标题，工具会校验并规范化；shot_output 必须填写语义 selector，例如 shot_01.preview_image.current 或 shot_02.shot_video.current。不要编造内部 ID 或 selector。"`
	ContentType    string `json:"content_type" jsonschema:"required,enum=image_url,enum=video_url,enum=audio_url" jsonschema_description:"官方 Seedance content item type。图片必须填 image_url；视频参考填 video_url；音频参考填 audio_url。"`
	ModelRole      string `json:"model_role" jsonschema:"required,enum=first_frame,enum=last_frame,enum=reference_image,enum=reference_video,enum=reference_audio" jsonschema_description:"官方 Seedance content.role。image_url 只能用 first_frame、last_frame 或 reference_image；video_url 只能用 reference_video；audio_url 只能用 reference_audio。商品/场景/风格语义写入 semantic_target 或 notes，不要写进 model_role。"`
	PromptAlias    string `json:"prompt_alias" jsonschema_description:"PromptCompiler 使用的可读别名，例如 图片1、视频1、音频1。不要手写 @图片1，交给编译器生成。"`
	SemanticTarget string `json:"semantic_target" jsonschema_description:"该参考约束的对象，例如悦行行李箱外观、机场出发大厅空间、上一个分镜尾帧。"`
	Priority       int    `json:"priority" jsonschema_description:"参考优先级，1 最高。重要素材应优先。"`
	Required       bool   `json:"required" jsonschema_description:"是否必须使用。商品、人脸、首帧、尾帧等关键参考通常为 true。"`
	Notes          string `json:"notes" jsonschema_description:"如何使用该参考的简短说明。不要写 provider prompt 语法。"`
}

type SubjectBindingInput struct {
	SubjectKey     string   `json:"subject_key" jsonschema:"required" jsonschema_description:"主体稳定键，例如 subject_luggage。"`
	Label          string   `json:"label" jsonschema:"required" jsonschema_description:"主体展示名，例如悦行银灰色行李箱。"`
	ElementStateID string   `json:"element_state_id" jsonschema_description:"兼容旧字段：对应 KeyElementState 内部 ID。没有则为空，模型通常不要填写。"`
	PromptHandle   string   `json:"prompt_handle" jsonschema_description:"主体句柄，例如 主体1。不要加尖括号，PromptCompiler 会渲染为 <主体1>。"`
	StableTraits   []string `json:"stable_traits" jsonschema_description:"2 到 5 个稳定静态特征，例如银灰色硬壳、竖向拉杆、四个万向轮。"`
	MustPreserve   bool     `json:"must_preserve" jsonschema_description:"是否必须保持一致。商品、人物通常为 true。"`
	AmbiguityNotes string   `json:"ambiguity_notes" jsonschema_description:"主体可能混淆的地方，例如不要变成黑色箱体或软布旅行袋。"`
}

type RenderPromptPartsInput struct {
	Objective      string   `json:"objective" jsonschema_description:"可选高级字段。本次生成目标；通常留空，让工具使用 generation_text。"`
	Subject        string   `json:"subject" jsonschema_description:"主体描述。应引用 subject binding 的语义，不写裸 ID。"`
	Setting        string   `json:"setting" jsonschema_description:"场景环境、时间、空间、光线。"`
	Action         string   `json:"action" jsonschema_description:"主体动作或事件。视频中应具体到可见动作。"`
	Camera         string   `json:"camera" jsonschema_description:"镜头语言和运镜。Seedance 视频应坚持一镜一主要运镜。"`
	Composition    string   `json:"composition" jsonschema_description:"构图、景别、主体位置、视觉焦点。"`
	Style          string   `json:"style" jsonschema_description:"整体风格、色彩、材质、商业质感。"`
	Lighting       string   `json:"lighting" jsonschema_description:"光影描述。与 ProjectMemory 视觉锚点保持一致。"`
	Sequence       []string `json:"sequence" jsonschema_description:"视频事件顺序。用于 Seedance 时按发生顺序描述，不写绝对秒数。"`
	Dialogue       string   `json:"dialogue" jsonschema_description:"台词文本。PromptCompiler 会按模型音频规则格式化。没有则为空。"`
	Narration      string   `json:"narration" jsonschema_description:"旁白文本。没有则为空。"`
	Audio          string   `json:"audio" jsonschema_description:"BGM、环境音、音效的创意说明。"`
	TextRendering  string   `json:"text_rendering" jsonschema_description:"画面中需要出现的文字、字幕或标题。没有则为空；不要滥加文字。"`
	QualityPack    []string `json:"quality_pack" jsonschema_description:"质量要求短句，例如高清、商业广告质感、稳定画面。不要堆砌过长约束。"`
	ConstraintPack []string `json:"constraint_pack" jsonschema_description:"硬约束短句，来自 ProjectMemory、用户要求和模型常见问题兜底。"`
	NegativeHints  []string `json:"negative_hints" jsonschema_description:"避免项，例如不要竞品 Logo、不要改变行李箱颜色。"`
}

type RenderPlanParamsInput struct {
	Ratio                     string           `json:"ratio" jsonschema_description:"输出比例，例如 9:16、16:9、1:1。未知时可为空并由 profile 默认。"`
	DurationSec               float64          `json:"duration_sec" jsonschema_description:"视频时长，单位秒。Seedance 当前只支持 5 或 10 秒；motion_shot_video 默认 5 秒，可用于低成本卖点卡片、商品图动效或 CTA；图片计划为空或 0。"`
	Resolution                string           `json:"resolution" jsonschema_description:"分辨率档位，例如 1080p、2K、4K。必须符合模型能力。"`
	Watermark                 bool             `json:"watermark" jsonschema_description:"是否添加水印。生产广告通常 false，除非配置要求。"`
	Speaker                   string           `json:"speaker" jsonschema_description:"seed-audio-1.0 音色 ID。voiceover_audio 通常需要填写；bgm_audio 通常为空。"`
	Format                    string           `json:"format" jsonschema_description:"音频输出格式，例如 mp3、wav、pcm、ogg_opus。"`
	SampleRate                int              `json:"sample_rate" jsonschema_description:"音频采样率，例如 48000。"`
	SpeechRate                float64          `json:"speech_rate" jsonschema_description:"语速调节参数。"`
	PitchRate                 float64          `json:"pitch_rate" jsonschema_description:"音调调节参数。"`
	LoudnessRate              float64          `json:"loudness_rate" jsonschema_description:"音量调节参数。"`
	GenerateAudio             bool             `json:"generate_audio" jsonschema_description:"Seedance 是否生成音频。没有明确音频计划时默认 false。"`
	ReturnLastFrame           bool             `json:"return_last_frame" jsonschema_description:"是否返回尾帧。last_frame_chain 的上游视频通常需要 true。"`
	CameraFixed               bool             `json:"camera_fixed" jsonschema_description:"是否固定镜头。与 camera prompt 冲突时工具应返回错误。"`
	FPS                       int              `json:"fps" jsonschema_description:"motion_shot_video 帧率，当前默认 30。"`
	Variables                 map[string]any   `json:"variables" jsonschema_description:"可选结构化变量，例如 headline、caption、cta。motion_shot_video 优先使用 text_layers 和 visual_layers。"`
	MotionStyle               string           `json:"motion_style" jsonschema_description:"motion_shot_video 动效风格，例如 premium_product_ad、social_fast、calm。"`
	SafeArea                  string           `json:"safe_area" jsonschema_description:"motion_shot_video 安全区策略，例如 caption_safe_bottom、center_product_safe。"`
	VisualLayers              []map[string]any `json:"visual_layers" jsonschema_description:"motion_shot_video 视觉层配置。只引用真实 input_ref，不写本地路径或代码。"`
	TextLayers                []map[string]any `json:"text_layers" jsonschema_description:"motion_shot_video 短文案层配置。只放大字标题和画面短文案，不放完整口播字幕。"`
	Transitions               map[string]any   `json:"transitions" jsonschema_description:"motion_shot_video 入场/出场转场配置，例如 in=soft_zoom、out=swipe_up。"`
	BrandColors               []string         `json:"brand_colors" jsonschema_description:"品牌色数组，使用 #RRGGBB。"`
	SequentialImageGeneration string           `json:"sequential_image_generation" jsonschema:"enum=auto,enum=disabled" jsonschema_description:"Seedream 组图能力。只在需要生成多张连续图片时使用 auto。"`
	MaxImages                 int              `json:"max_images" jsonschema_description:"Seedream 组图数量。单张参考图通常为 1。"`
	Seed                      int64            `json:"seed" jsonschema_description:"可选随机种子。没有明确复现需求时留空或 0。"`
}

type RenderPlanAuditHintsInput struct {
	AutoFilled          []string `json:"auto_filled" jsonschema_description:"Craftsman 合理补全的非关键内容，例如机场晨光色温。"`
	NeedsUserDecision   []string `json:"needs_user_decision" jsonschema_description:"需要 Producer 询问用户的关键歧义。Craftsman 不能直接问用户。"`
	CapabilityRisks     []string `json:"capability_risks" jsonschema_description:"模型能力或成本风险，例如人物参考过多、视频时长过长。"`
	ConsistencyRisks    []string `json:"consistency_risks" jsonschema_description:"一致性风险，例如缺少商品侧面参考。"`
	PromptCompilerNotes []string `json:"prompt_compiler_notes" jsonschema_description:"给 PromptCompiler 的短提示，例如优先绑定商品图为图片1。"`
}

type RenderPlanBlockerInput struct {
	BlockerType string   `json:"blocker_type" jsonschema:"enum=missing_reference,enum=dependency_not_ready,enum=memory_conflict,enum=model_capability,enum=ambiguous_instruction,enum=invalid_scope" jsonschema_description:"阻塞类型。"`
	Message     string   `json:"message" jsonschema_description:"阻塞原因，必须给 Producer 看得懂。"`
	NeededBy    string   `json:"needed_by" jsonschema_description:"阻塞影响的阶段，例如 preview_image 或 shot_video。"`
	Suggestions []string `json:"suggestions" jsonschema_description:"建议 Producer 下一步怎么做，例如先生成机场 KeyElementState reference image。"`
}

func NewUpsertRenderPlanNativeTool(service *renderplan.Service, submitter ...*RenderPlanSubmitter) *UpsertRenderPlanNativeTool {
	var configuredSubmitter *RenderPlanSubmitter
	if len(submitter) > 0 {
		configuredSubmitter = submitter[0]
	}
	return &UpsertRenderPlanNativeTool{service: service, submitter: configuredSubmitter}
}

func (t *UpsertRenderPlanNativeTool) WithReferenceStore(store RenderPlanReferenceStore) *UpsertRenderPlanNativeTool {
	t.referenceStore = store
	return t
}

func (t *UpsertRenderPlanNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[UpsertRenderPlanToolInput](toolUpsertRenderPlan, "创建、更新草稿或 fork 一个 ClipAnvil RenderPlan，用于 Seedream 图片和 Seedance 视频生成。优先使用 generation_text 写完整自然语言生成说明；scope、target_phase、model_prompt_profile、operation 和 task_type 通常由当前 Craftsman task 与引用资源自动推导。只填写必要 reference_bindings 和 params，不要把创作描述拆成超长多层 JSON。")
}

func (t *UpsertRenderPlanNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs[UpsertRenderPlanToolInput](toolUpsertRenderPlan, argumentsInJSON, nil)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolUpsertRenderPlan)
	if !ok {
		return msg, nil
	}
	input = applyUpsertRenderPlanRuntimeDefaults(input, runtime)
	if msg, ok := validateUpsertRenderPlanRuntime(runtime, input); !ok {
		return msg, nil
	}
	if err := validateUpsertRenderPlanInput(input); err != nil {
		return NaturalToolError(toolUpsertRenderPlan, err.Error(), "请修正参数后重试，不要重复提交相同错误参数。"), nil
	}
	if normalized, msg, ok := t.validateAndNormalizeReferenceBindings(ctx, runtime, input); !ok {
		return msg, nil
	} else {
		input = normalized
	}
	if msg, ok := renderPlanServiceOrError(t.service, toolUpsertRenderPlan); !ok {
		return msg, nil
	}
	scopeID, _ := pgUUIDFromString(input.Scope.ID)
	renderPlanID, _ := pgUUIDFromString(input.RenderPlanID)
	forkID, _ := pgUUIDFromString(input.ForkFromRenderPlanID)
	out, err := t.service.Upsert(ctx, renderplan.UpsertInput{
		WorkspaceID:          runtime.WorkspaceID,
		ThreadID:             runtime.ThreadID,
		TaskID:               runtime.TaskID,
		Brief:                input.Brief,
		Mode:                 input.Mode,
		RenderPlanID:         renderPlanID,
		ForkFromRenderPlanID: forkID,
		Scope:                renderplan.Scope{Type: input.Scope.Type, ID: scopeID, Key: input.Scope.Key},
		TargetPhase:          input.TargetPhase,
		TaskType:             input.TaskType,
		ModelPromptProfile:   input.ModelPromptProfile,
		Operation:            input.Operation,
		ReferenceBindings:    toRenderPlanReferenceBindings(input.ReferenceBindings),
		SubjectBindings:      toRenderPlanSubjectBindings(input.SubjectBindings),
		PromptParts:          toRenderPlanPromptParts(input.PromptParts),
		Params:               toRenderPlanParams(input.Params),
		AuditHints:           toRenderPlanAuditHints(input.AuditHints),
		Blocker:              toRenderPlanBlocker(input.Blocker),
		Rationale:            input.Rationale,
		ExecutionPolicy:      renderPlanExecutionPolicy(runtime.ExecutionPolicy),
	})
	if err != nil {
		return naturalErrorFromErr(toolUpsertRenderPlan, err), nil
	}
	var workerTask dbAgentTaskSummary
	if renderPlanExecutionPolicy(runtime.ExecutionPolicy) == renderplan.ExecutionPolicyExecuteImmediately {
		if t.submitter == nil {
			return NaturalToolError(toolUpsertRenderPlan, "RenderPlan 已编译，但 submitter 未配置，无法直接提交 Worker。", "请让 Producer 使用 wait_for_producer 策略，或检查服务端 wiring。"), nil
		}
		task, submitted, err := t.submitter.SubmitRenderPlan(ctx, runtime.WorkspaceID, out.ID, runtime.ThreadID, "execution_policy=execute_immediately")
		if err != nil {
			return NaturalToolError(toolUpsertRenderPlan, "RenderPlan 已编译，但提交 Worker 失败："+err.Error(), "请让 Producer 读取 RenderPlan 状态后使用 decide_render_plan 重试。"), nil
		}
		out = submitted
		workerTask = dbAgentTaskSummary{Ref: agentTaskResultLabel(task), Status: task.Status}
	}
	title := "已写入 RenderPlan"
	if out.Status == renderplan.StatusBlocked {
		title = "RenderPlan 已标记为 blocked"
	}
	items := []NaturalResultItem{
		{Label: "RenderPlan", Value: renderPlanDecisionLabel(out)},
		{Label: "Scope", Value: renderPlanScopeLabel(out)},
		{Label: "阶段", Value: out.TargetPhase},
		{Label: "Profile", Value: out.ModelPromptProfile},
		{Label: "Operation", Value: out.Operation},
		{Label: "状态", Value: out.Status},
		{Label: "CompiledPrompt", Value: fmt.Sprintf("%d 字符", len([]rune(out.CompiledPrompt)))},
	}
	next := "Producer 可读取项目上下文并决定 accept/reject 或派 Reviewer。"
	if workerTask.Ref != "" {
		items = append(items, NaturalResultItem{Label: "WorkerTask", Value: workerTask.Ref + " / " + workerTask.Status})
		next = "已根据 execute_immediately 提交 worker_generation。Producer 后续应读取 generation_job、artifact_version 和 RenderPlan 状态，不要把提交等同于产物完成。"
	}
	return NaturalResult{
		Title: title,
		Items: items,
		Next:  next,
	}.String(), nil
}

type dbAgentTaskSummary struct {
	Ref    string
	Status string
}

func renderPlanScopeLabel(plan db.RenderPlan) string {
	semanticKey := strings.TrimSpace(plan.SemanticKey)
	targetPhase := strings.TrimSpace(plan.TargetPhase)
	if semanticKey != "" && targetPhase != "" {
		marker := "." + targetPhase + "."
		if index := strings.Index(semanticKey, marker); index > 0 {
			return plan.ScopeType + "=" + semanticKey[:index]
		}
	}
	return plan.ScopeType + "=semantic_key_missing"
}

func renderPlanExecutionPolicy(value string) string {
	switch value {
	case renderplan.ExecutionPolicyExecuteImmediately:
		return renderplan.ExecutionPolicyExecuteImmediately
	default:
		return renderplan.ExecutionPolicyWaitForProducer
	}
}

func validateUpsertRenderPlanInput(input UpsertRenderPlanToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.Mode, "create", "update_draft", "fork_from", "mark_blocked"); err != nil {
		return err
	}
	if err := requireText(input.Scope.Type, "scope.type"); err != nil {
		return err
	}
	if err := requireMode(input.Scope.Type, "key_element_state", "shot", "audio_plan"); err != nil {
		return err
	}
	if _, ok := pgUUIDFromString(input.Scope.ID); !ok {
		return fmt.Errorf("scope.id 必须继承当前 Craftsman task 的内部 ID；模型通常不要填写")
	}
	if input.RenderPlanID != "" {
		if _, ok := pgUUIDFromString(input.RenderPlanID); !ok {
			return fmt.Errorf("render_plan_id 必须是系统已有 RenderPlan 的内部 ID；模型通常不要填写")
		}
	}
	if input.ForkFromRenderPlanID != "" {
		if _, ok := pgUUIDFromString(input.ForkFromRenderPlanID); !ok {
			return fmt.Errorf("fork_from_render_plan_id 必须是系统已有 RenderPlan 的内部 ID；模型通常不要填写")
		}
	}
	if err := requireMode(input.TargetPhase, "reference_image", "preview_image", "shot_video", "voiceover_audio", "bgm_audio"); err != nil {
		return err
	}
	if err := requireMode(input.TaskType, "generate", "edit", "extend", "bridge"); err != nil {
		return err
	}
	if err := requireMode(input.ModelPromptProfile, "seedream_5_image", "seedance_2_video", "seed_audio_1", "motion_shot_video"); err != nil {
		return err
	}
	if err := requireMode(input.Operation, "text_to_image", "image_to_image", "multi_image_to_image", "text_to_video", "image_to_video_first_frame", "image_to_video_first_last_frame", "multi_modal_reference_video", "video_edit", "video_extend", "video_bridge", "text_to_audio", "image_to_motion_video"); err != nil {
		return err
	}
	if err := validateReferenceBindingContent(input.ReferenceBindings, input.Operation); err != nil {
		return err
	}
	if input.Mode == "mark_blocked" {
		if input.Blocker.BlockerType == "" || input.Blocker.Message == "" {
			return fmt.Errorf("mark_blocked 需要 blocker.blocker_type 和 blocker.message")
		}
		return nil
	}
	if err := requireText(input.GenerationText, "generation_text"); err != nil {
		return err
	}
	return nil
}

func validateReferenceBindingContent(bindings []ReferenceBindingInput, operation string) error {
	for index, binding := range bindings {
		if strings.TrimSpace(binding.ContentType) == "" {
			return fmt.Errorf("reference_bindings[%d].content_type 必填", index)
		}
		if strings.TrimSpace(binding.ModelRole) == "" {
			return fmt.Errorf("reference_bindings[%d].model_role 必填", index)
		}
		if err := validateContentModelRole(binding.ContentType, binding.ModelRole); err != nil {
			return fmt.Errorf("reference_bindings[%d]: %w", index, err)
		}
	}
	switch strings.TrimSpace(operation) {
	case "image_to_video_first_frame":
		if len(bindings) != 1 || bindings[0].ContentType != "image_url" || bindings[0].ModelRole != "first_frame" {
			return fmt.Errorf("image_to_video_first_frame 需要且只需要 1 个 reference_binding: content_type=image_url, model_role=first_frame")
		}
	case "image_to_video_first_last_frame":
		firstFrames, lastFrames := 0, 0
		for _, binding := range bindings {
			if binding.ContentType != "image_url" {
				return fmt.Errorf("image_to_video_first_last_frame 只能使用 image_url")
			}
			switch binding.ModelRole {
			case "first_frame":
				firstFrames++
			case "last_frame":
				lastFrames++
			default:
				return fmt.Errorf("image_to_video_first_last_frame 只能使用 model_role=first_frame 或 last_frame")
			}
		}
		if len(bindings) != 2 || firstFrames != 1 || lastFrames != 1 {
			return fmt.Errorf("image_to_video_first_last_frame 必须提供 2 个 image_url，分别是 first_frame 和 last_frame")
		}
	case "multi_modal_reference_video":
		if len(bindings) == 0 {
			return fmt.Errorf("multi_modal_reference_video 至少需要 1 个 reference_binding")
		}
		imageRefs := 0
		for _, binding := range bindings {
			switch binding.ModelRole {
			case "reference_image":
				imageRefs++
			case "reference_video", "reference_audio":
			default:
				return fmt.Errorf("multi_modal_reference_video 只能使用 reference_image、reference_video 或 reference_audio")
			}
		}
		if imageRefs > 9 {
			return fmt.Errorf("multi_modal_reference_video 最多支持 9 张 reference_image")
		}
	}
	return nil
}

func validateContentModelRole(contentType string, modelRole string) error {
	switch strings.TrimSpace(contentType) {
	case "image_url":
		if modelRole == "first_frame" || modelRole == "last_frame" || modelRole == "reference_image" {
			return nil
		}
		return fmt.Errorf("image_url 只能使用 model_role=first_frame、last_frame 或 reference_image")
	case "video_url":
		if modelRole == "reference_video" {
			return nil
		}
		return fmt.Errorf("video_url 只能使用 model_role=reference_video")
	case "audio_url":
		if modelRole == "reference_audio" {
			return nil
		}
		return fmt.Errorf("audio_url 只能使用 model_role=reference_audio")
	default:
		return fmt.Errorf("content_type 只能是 image_url、video_url 或 audio_url")
	}
}

func validateUpsertRenderPlanRuntime(runtime NativeRuntimeContext, input UpsertRenderPlanToolInput) (string, bool) {
	if strings.TrimSpace(runtime.ScopeType) != "" && input.Scope.Type != runtime.ScopeType {
		return NaturalToolError(toolUpsertRenderPlan, fmt.Sprintf("scope 必须与当前 Craftsman 任务一致：当前是 %s，工具参数是 %s", runtime.ScopeType, input.Scope.Type), "请读取当前 Craftsman 任务上下文，使用 dispatch_craftsman 传入的 scope。"), false
	}
	if runtime.ScopeID.Valid {
		scopeID, _ := pgUUIDFromString(input.Scope.ID)
		if scopeID != runtime.ScopeID {
			return NaturalToolError(toolUpsertRenderPlan, fmt.Sprintf("scope 必须与当前 Craftsman 任务一致：当前 scope_id=%s，工具参数 scope.id=%s", uuidString(runtime.ScopeID), input.Scope.ID), "请不要跨分镜或跨关键元素写 RenderPlan；如需处理其他 scope，请让 Producer 另行派发 Craftsman。"), false
		}
	}
	if strings.TrimSpace(runtime.TargetPhase) != "" && input.TargetPhase != runtime.TargetPhase {
		return NaturalToolError(toolUpsertRenderPlan, fmt.Sprintf("target_phase 必须与当前 Craftsman 任务一致：当前是 %s，工具参数是 %s", runtime.TargetPhase, input.TargetPhase), "请继承 dispatch_craftsman 的 target_phase。preview_image 任务只能写 seedream 预览图计划，不能改成 shot_video。"), false
	}
	if isMotionOnlyVideoPolicy(runtime.VideoRoutePolicy) && input.TargetPhase == renderplan.PhaseShotVideo && input.Mode != "mark_blocked" {
		if input.ModelPromptProfile != renderplan.ProfileMotionShotVideo || !isMotionVideoOperation(input.Operation) {
			return NaturalToolError(toolUpsertRenderPlan, fmt.Sprintf("当前任务设置 video_route_policy=motion_only，禁止 Seedance；收到 profile=%s operation=%s", input.ModelPromptProfile, input.Operation), "请改用 model_prompt_profile=motion_shot_video，并使用 operation=image_to_motion_video；如果无法生成图片动效视频，请用 mark_blocked 说明原因。"), false
		}
	}
	return "", true
}

func normalizeUpsertRenderPlanInput(input UpsertRenderPlanToolInput) UpsertRenderPlanToolInput {
	generationText := strings.TrimSpace(input.GenerationText)
	if generationText != "" {
		input.GenerationText = generationText
		input.PromptParts.Objective = generationText
		if input.TargetPhase == renderplan.PhaseShotVideo && strings.TrimSpace(input.PromptParts.Action) == "" && len(input.PromptParts.Sequence) == 0 {
			input.PromptParts.Action = generationText
		}
	}
	if strings.TrimSpace(input.PromptParts.Objective) == "" {
		input.PromptParts.Objective = strings.TrimSpace(input.Brief)
	}
	if strings.TrimSpace(input.Rationale) == "" {
		input.Rationale = strings.TrimSpace(input.Brief)
	}
	return input
}

func applyUpsertRenderPlanRuntimeDefaults(input UpsertRenderPlanToolInput, runtime NativeRuntimeContext) UpsertRenderPlanToolInput {
	if strings.TrimSpace(input.Scope.Type) == "" {
		input.Scope.Type = strings.TrimSpace(runtime.ScopeType)
	}
	if strings.TrimSpace(input.Scope.ID) == "" && runtime.ScopeID.Valid {
		input.Scope.ID = uuidString(runtime.ScopeID)
	}
	if strings.TrimSpace(input.Scope.Key) == "" {
		input.Scope.Key = strings.TrimSpace(runtime.ScopeKey)
	}
	if strings.TrimSpace(input.TargetPhase) == "" {
		input.TargetPhase = strings.TrimSpace(runtime.TargetPhase)
	}
	if strings.TrimSpace(input.TaskType) == "" {
		input.TaskType = renderplan.TaskGenerate
	}
	motionOnlyShotVideo := isMotionOnlyVideoPolicy(runtime.VideoRoutePolicy) && input.TargetPhase == renderplan.PhaseShotVideo
	if strings.TrimSpace(input.ModelPromptProfile) == "" {
		if motionOnlyShotVideo {
			input.ModelPromptProfile = renderplan.ProfileMotionShotVideo
		} else {
			input.ModelPromptProfile = firstNonEmptyString(runtime.RecommendedModelPromptProfile, inferRenderPlanProfile(input.TargetPhase))
		}
	}
	if len(input.ReferenceBindings) == 0 && len(runtime.InputNodeRefs) > 0 {
		input.ReferenceBindings = referenceBindingsFromRuntimeInputRefs(runtime.InputNodeRefs, input.ModelPromptProfile)
	}
	if strings.TrimSpace(input.Operation) == "" {
		if motionOnlyShotVideo {
			input.Operation = firstNonEmptyString(runtime.RecommendedOperation, "image_to_motion_video")
		} else {
			input.Operation = firstNonEmptyString(runtime.RecommendedOperation, inferRenderPlanOperation(input.TargetPhase, input.ReferenceBindings))
		}
	}
	input.Params = applyRecommendedRenderPlanParams(input.Params, runtime.RecommendedParams)
	return normalizeUpsertRenderPlanInput(input)
}

func isMotionVideoOperation(operation string) bool {
	switch strings.TrimSpace(operation) {
	case "image_to_motion_video":
		return true
	default:
		return false
	}
}

func inferRenderPlanProfile(targetPhase string) string {
	switch strings.TrimSpace(targetPhase) {
	case renderplan.PhaseVoiceoverAudio, renderplan.PhaseBGMAudio:
		return renderplan.ProfileSeedAudio1
	case renderplan.PhaseShotVideo:
		return renderplan.ProfileSeedance2Video
	case renderplan.PhaseReferenceImage, renderplan.PhasePreviewImage:
		return renderplan.ProfileSeedream5Image
	default:
		return ""
	}
}

func inferRenderPlanOperation(targetPhase string, bindings []ReferenceBindingInput) string {
	switch strings.TrimSpace(targetPhase) {
	case renderplan.PhaseVoiceoverAudio, renderplan.PhaseBGMAudio:
		return "text_to_audio"
	case renderplan.PhaseShotVideo:
		hasFirstFrame := false
		hasLastFrame := false
		for _, binding := range bindings {
			switch strings.TrimSpace(binding.ModelRole) {
			case "first_frame":
				hasFirstFrame = true
			case "last_frame":
				hasLastFrame = true
			}
		}
		if hasFirstFrame && hasLastFrame {
			return "image_to_video_first_last_frame"
		}
		if hasFirstFrame {
			return "image_to_video_first_frame"
		}
		if len(bindings) > 0 {
			return "multi_modal_reference_video"
		}
		return "text_to_video"
	case renderplan.PhaseReferenceImage, renderplan.PhasePreviewImage:
		if len(bindings) > 1 {
			return "multi_image_to_image"
		}
		if len(bindings) == 1 {
			return "image_to_image"
		}
		return "text_to_image"
	default:
		return ""
	}
}

func referenceBindingsFromRuntimeInputRefs(inputRefs []string, profile string) []ReferenceBindingInput {
	out := make([]ReferenceBindingInput, 0, len(inputRefs))
	for index, ref := range inputRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		modelRole := "first_frame"
		if profile == renderplan.ProfileMotionShotVideo {
			modelRole = "reference_image"
		} else if len(inputRefs) == 2 && index == 1 {
			modelRole = "last_frame"
		} else if len(inputRefs) > 2 {
			modelRole = "reference_image"
		}
		out = append(out, ReferenceBindingInput{
			ClientKey:      fmt.Sprintf("ref_task_input_%d", len(out)+1),
			SourceType:     "media_node",
			SourceID:       ref,
			ContentType:    "image_url",
			ModelRole:      modelRole,
			SemanticTarget: "current task input",
			Required:       true,
		})
	}
	return out
}

func applyRecommendedRenderPlanParams(input RenderPlanParamsInput, recommended map[string]any) RenderPlanParamsInput {
	if len(recommended) == 0 {
		return input
	}
	if input.Ratio == "" {
		input.Ratio = stringValue(recommended, "ratio")
	}
	if input.DurationSec == 0 {
		input.DurationSec = float64Value(recommended, "duration_sec")
	}
	if input.Resolution == "" {
		input.Resolution = stringValue(recommended, "resolution")
	}
	if input.FPS == 0 {
		input.FPS = int(int32Value(recommended, "fps", 0))
	}
	if input.MotionStyle == "" {
		input.MotionStyle = stringValue(recommended, "motion_style")
	}
	if input.SafeArea == "" {
		input.SafeArea = stringValue(recommended, "safe_area")
	}
	if input.Variables == nil {
		if variables, ok := recommended["variables"].(map[string]any); ok {
			input.Variables = variables
		}
	}
	if input.Transitions == nil {
		if transitions, ok := recommended["transitions"].(map[string]any); ok {
			input.Transitions = transitions
		}
	}
	return input
}

func float64Value(raw map[string]any, key string) float64 {
	value, ok := raw[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func (t *UpsertRenderPlanNativeTool) validateAndNormalizeReferenceBindings(ctx context.Context, runtime NativeRuntimeContext, input UpsertRenderPlanToolInput) (UpsertRenderPlanToolInput, string, bool) {
	if input.Mode == "mark_blocked" || !hasMediaNodeReferenceBinding(input.ReferenceBindings) {
		return input, "", true
	}
	if t.referenceStore == nil {
		return input, NaturalToolError(toolUpsertRenderPlan, "reference_bindings 包含 media_node，但工具缺少 media_node 校验 store，无法确认引用是否属于当前 workspace。", "请检查服务端 wiring；不要跳过引用校验直接提交 Worker。"), false
	}
	nodes, err := t.referenceStore.ListMediaNodesByWorkspace(ctx, runtime.WorkspaceID)
	if err != nil {
		return input, NaturalToolError(toolUpsertRenderPlan, "读取当前 workspace 的 media_node 列表失败："+err.Error(), "请稍后重试；如果持续失败，请让工程侧检查数据库连接和 workspace_id。"), false
	}
	normalized, problem, ok := normalizeMediaNodeReferenceBindings(input, nodes)
	if !ok {
		return input, NaturalToolError(toolUpsertRenderPlan, problem, "请先读取项目上下文，使用当前 workspace 中真实存在的 media_node semantic_key；如果只知道素材标题，必须填写唯一标题，不要编造内部 ID。"), false
	}
	return normalized, "", true
}

func hasMediaNodeReferenceBinding(bindings []ReferenceBindingInput) bool {
	for _, binding := range bindings {
		if strings.TrimSpace(binding.SourceType) == "media_node" {
			return true
		}
	}
	return false
}

func normalizeMediaNodeReferenceBindings(input UpsertRenderPlanToolInput, nodes []db.MediaNode) (UpsertRenderPlanToolInput, string, bool) {
	for i, binding := range input.ReferenceBindings {
		if strings.TrimSpace(binding.SourceType) != "media_node" {
			continue
		}
		sourceID := strings.TrimSpace(binding.SourceID)
		if sourceID == "" {
			return input, fmt.Sprintf("reference_bindings[%d].source_id 为空，无法绑定 media_node。当前可用 media_node：%s", i, describeAvailableMediaNodes(nodes)), false
		}
		node, problem := resolveRenderPlanMediaNodeReference(nodes, sourceID)
		if problem != "" {
			return input, fmt.Sprintf("reference_bindings[%d].source_id=%q %s。当前可用 media_node：%s", i, sourceID, problem, describeAvailableMediaNodes(nodes)), false
		}
		input.ReferenceBindings[i].SourceID = uuidString(node.ID)
	}
	return input, "", true
}

func resolveRenderPlanMediaNodeReference(nodes []db.MediaNode, sourceID string) (db.MediaNode, string) {
	if id, ok := pgUUIDFromString(sourceID); ok {
		for _, node := range nodes {
			if node.ID == id {
				return node, ""
			}
		}
		return db.MediaNode{}, "指向的 media_node 不存在"
	}
	var matches []db.MediaNode
	for _, node := range nodes {
		if strings.EqualFold(strings.TrimSpace(node.Title), sourceID) {
			matches = append(matches, node)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], ""
	case 0:
		return db.MediaNode{}, "没有匹配到同名 media_node"
	default:
		return db.MediaNode{}, "匹配到多个同名 media_node，引用有歧义"
	}
}

func describeAvailableMediaNodes(nodes []db.MediaNode) string {
	if len(nodes) == 0 {
		return "当前 workspace 没有可用 media_node"
	}
	limit := len(nodes)
	if limit > 8 {
		limit = 8
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		title := strings.TrimSpace(nodes[i].Title)
		if title == "" {
			title = "(无标题)"
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", title, uuidString(nodes[i].ID)))
	}
	if len(nodes) > limit {
		parts = append(parts, fmt.Sprintf("另有 %d 个", len(nodes)-limit))
	}
	return strings.Join(parts, "、")
}

func toRenderPlanReferenceBindings(input []ReferenceBindingInput) []renderplan.ReferenceBinding {
	out := make([]renderplan.ReferenceBinding, 0, len(input))
	for _, item := range input {
		out = append(out, renderplan.ReferenceBinding{ClientKey: item.ClientKey, SourceType: item.SourceType, SourceID: item.SourceID, ContentType: item.ContentType, ModelRole: item.ModelRole, PromptAlias: item.PromptAlias, SemanticTarget: item.SemanticTarget, Priority: item.Priority, Required: item.Required, Notes: item.Notes})
	}
	return out
}

func toRenderPlanSubjectBindings(input []SubjectBindingInput) []renderplan.SubjectBinding {
	out := make([]renderplan.SubjectBinding, 0, len(input))
	for _, item := range input {
		out = append(out, renderplan.SubjectBinding{SubjectKey: item.SubjectKey, Label: item.Label, ElementStateID: item.ElementStateID, PromptHandle: item.PromptHandle, StableTraits: item.StableTraits, MustPreserve: item.MustPreserve, AmbiguityNotes: item.AmbiguityNotes})
	}
	return out
}

func toRenderPlanPromptParts(input RenderPromptPartsInput) renderplan.PromptParts {
	return renderplan.PromptParts{Objective: input.Objective, Subject: input.Subject, Setting: input.Setting, Action: input.Action, Camera: input.Camera, Composition: input.Composition, Style: input.Style, Lighting: input.Lighting, Sequence: input.Sequence, Dialogue: input.Dialogue, Narration: input.Narration, Audio: input.Audio, TextRendering: input.TextRendering, QualityPack: input.QualityPack, ConstraintPack: input.ConstraintPack, NegativeHints: input.NegativeHints}
}

func toRenderPlanParams(input RenderPlanParamsInput) renderplan.Params {
	return renderplan.Params{
		Ratio:                     input.Ratio,
		DurationSec:               input.DurationSec,
		Resolution:                input.Resolution,
		Watermark:                 input.Watermark,
		Speaker:                   input.Speaker,
		Format:                    input.Format,
		SampleRate:                input.SampleRate,
		SpeechRate:                input.SpeechRate,
		PitchRate:                 input.PitchRate,
		LoudnessRate:              input.LoudnessRate,
		GenerateAudio:             input.GenerateAudio,
		ReturnLastFrame:           input.ReturnLastFrame,
		CameraFixed:               input.CameraFixed,
		FPS:                       input.FPS,
		Variables:                 input.Variables,
		MotionStyle:               input.MotionStyle,
		SafeArea:                  input.SafeArea,
		VisualLayers:              input.VisualLayers,
		TextLayers:                input.TextLayers,
		Transitions:               input.Transitions,
		BrandColors:               input.BrandColors,
		SequentialImageGeneration: input.SequentialImageGeneration,
		MaxImages:                 input.MaxImages,
		Seed:                      input.Seed,
	}
}

func toRenderPlanAuditHints(input RenderPlanAuditHintsInput) renderplan.AuditHints {
	return renderplan.AuditHints{AutoFilled: input.AutoFilled, NeedsUserDecision: input.NeedsUserDecision, CapabilityRisks: input.CapabilityRisks, ConsistencyRisks: input.ConsistencyRisks, PromptCompilerNotes: input.PromptCompilerNotes}
}

func toRenderPlanBlocker(input RenderPlanBlockerInput) renderplan.Blocker {
	return renderplan.Blocker{BlockerType: input.BlockerType, Message: input.Message, NeededBy: input.NeededBy, Suggestions: input.Suggestions}
}
