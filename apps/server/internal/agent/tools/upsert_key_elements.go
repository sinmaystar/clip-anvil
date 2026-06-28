package tools

import (
	"context"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
)

type UpsertKeyElementsNativeTool struct {
	service *creative.Service
}

type UpsertKeyElementsToolInput struct {
	Brief    string            `json:"brief" jsonschema:"required" jsonschema_description:"本次写入关键元素的业务目的，例如把上传行李箱和机场场景沉淀为可复用锚点。不要超过 160 个中文字符。"`
	Mode     string            `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=replace" jsonschema_description:"create 创建新元素；patch 更新已有元素或状态；replace 替换 Producer 管理的草稿元素集合。"`
	Elements []KeyElementInput `json:"elements" jsonschema:"required" jsonschema_description:"要创建或更新的关键元素列表。每个元素必须有稳定 client_key。"`
	Reason   string            `json:"reason" jsonschema_description:"为什么写入这些关键元素。"`
}

type KeyElementInput struct {
	ClientKey   string                 `json:"client_key" jsonschema:"required" jsonschema_description:"模型可稳定引用的业务键，例如 product_yuexing_luggage。批量 upsert 必须稳定。"`
	ElementType string                 `json:"element_type" jsonschema:"required,enum=product,enum=character,enum=scene,enum=prop,enum=style" jsonschema_description:"关键元素类型。product 商品；character 人物；scene 场景；prop 道具；style 风格参考。"`
	Name        string                 `json:"name" jsonschema:"required" jsonschema_description:"用户可读名称，例如悦行行李箱、机场出发大厅。"`
	Description string                 `json:"description" jsonschema_description:"元素整体说明，不要承载状态细节；具体视觉状态写入 states.visual_description。"`
	SourceType  string                 `json:"source_type" jsonschema:"enum=user_asset,enum=material_analysis,enum=prompt_derived,enum=agent_created" jsonschema_description:"元素来源。用户上传素材用 user_asset；模型素材分析用 material_analysis；用户文字提到但无素材用 prompt_derived；Agent 生成的参考资源或概念用 agent_created。"`
	SourceRefs  []ElementSourceRef     `json:"source_refs" jsonschema_description:"元素来源引用，例如素材节点、用户消息或 brief。"`
	States      []KeyElementStateInput `json:"states" jsonschema_description:"元素的视觉状态列表。至少应有一个默认状态，除非只是 patch 元素名称。"`
}

type KeyElementStateInput struct {
	ClientKey          string             `json:"client_key" jsonschema:"required" jsonschema_description:"状态稳定业务键，例如 state_uploaded_front、state_modern_morning。"`
	Label              string             `json:"label" jsonschema_description:"状态展示名，例如用户上传素材状态、现代机场晨光状态。"`
	VisualDescription  string             `json:"visual_description" jsonschema_description:"该状态的具体视觉描述。人物/商品/场景的一致性主要看这个字段。不要写模型 prompt 语法。"`
	ReferenceStatus    string             `json:"reference_status" jsonschema:"required,enum=none,enum=needs_reference,enum=ready,enum=approved,enum=rejected" jsonschema_description:"参考资源状态。none 表示这个状态只作为文字约束，不需要生成统一参考图；needs_reference 表示为了跨分镜一致性必须生成或上传参考；ready 表示已有可用参考；approved 表示用户确认；rejected 表示参考被否定。"`
	ReferenceNodeID    string             `json:"reference_node_id" jsonschema_description:"兼容旧字段：参考素材所在 media_node 内部 ID。needs_reference 时为空；模型通常不要填写。"`
	ReferenceVersionID string             `json:"reference_version_id" jsonschema_description:"兼容旧字段：被选中的 artifact_version 内部 ID。M1 通常为空；模型通常不要填写。"`
	IsDefault          bool               `json:"is_default" jsonschema_description:"是否为该 key element 默认状态。同一元素同一时间只能有一个默认状态。"`
	StateFacts         []MemoryFactInput  `json:"state_facts" jsonschema_description:"该状态的结构化事实，例如 color=silver、lighting=morning。"`
	SourceRefs         []ElementSourceRef `json:"source_refs" jsonschema_description:"该状态的来源引用。"`
}

type ElementSourceRef struct {
	Type string `json:"type" jsonschema:"required,enum=user_message,enum=media_node,enum=asset,enum=creative_brief" jsonschema_description:"来源类型。"`
	ID   string `json:"id" jsonschema_description:"来源对象 ID。"`
	Note string `json:"note" jsonschema_description:"来源说明，例如用户上传的行李箱图片。"`
}

func NewUpsertKeyElementsNativeTool(service *creative.Service) *UpsertKeyElementsNativeTool {
	return &UpsertKeyElementsNativeTool{service: service}
}

func (t *UpsertKeyElementsNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[UpsertKeyElementsToolInput](toolUpsertElements, "创建、局部更新或替换 ClipAnvil 关键元素 KeyElement 及其视觉状态 KeyElementState。关键元素是视频一致性的锚点，包括商品、人物、场景、道具和风格参考。只有需要跨分镜锁定外观/场景/人物时才把状态设为 needs_reference；普通文字约束可使用 reference_status=none。")
}

func (t *UpsertKeyElementsNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolUpsertElements, argumentsInJSON, validateUpsertKeyElementsInput)
	if !ok {
		return msg, nil
	}
	if msg, ok := serviceOrError(t.service, toolUpsertElements); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolUpsertElements)
	if !ok {
		return msg, nil
	}
	out, err := t.service.UpsertKeyElements(ctx, creative.UpsertKeyElementsInput{
		WorkspaceID: runtime.WorkspaceID,
		ThreadID:    runtime.ThreadID,
		TaskID:      runtime.TaskID,
		Brief:       input.Brief,
		Mode:        input.Mode,
		Elements:    toCreativeKeyElements(input.Elements),
		Reason:      input.Reason,
	})
	if err != nil {
		return naturalErrorFromErr(toolUpsertElements, err), nil
	}
	return NaturalResult{
		Title: "已更新关键元素",
		Items: []NaturalResultItem{
			{Label: "元素", Value: fmt.Sprintf("创建 %d 个，更新 %d 个", out.ElementsCreated, out.ElementsUpdated)},
			{Label: "状态", Value: fmt.Sprintf("创建 %d 个，更新 %d 个", out.StatesCreated, out.StatesUpdated)},
		},
		Next: "如果用户需要完整分镜，继续使用 upsert_storyboard 引用这些关键元素。",
	}.String(), nil
}

func validateUpsertKeyElementsInput(input UpsertKeyElementsToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.Mode, "create", "patch", "replace"); err != nil {
		return err
	}
	if len(input.Elements) == 0 {
		return fmt.Errorf("elements 必须至少包含一个关键元素")
	}
	for _, element := range input.Elements {
		if err := requireText(element.ClientKey, "elements.client_key"); err != nil {
			return err
		}
		if err := requireText(element.ElementType, "elements.element_type"); err != nil {
			return err
		}
		if err := requireText(element.Name, "elements.name"); err != nil {
			return err
		}
		if err := requireMode(element.ElementType, "product", "character", "scene", "prop", "style"); err != nil {
			return err
		}
	}
	return nil
}

func toCreativeKeyElements(input []KeyElementInput) []creative.KeyElementInput {
	out := make([]creative.KeyElementInput, 0, len(input))
	for _, element := range input {
		out = append(out, creative.KeyElementInput{
			ClientKey:   element.ClientKey,
			ElementType: element.ElementType,
			Name:        element.Name,
			Description: element.Description,
			SourceType:  element.SourceType,
			SourceRefs:  toCreativeElementSourceRefs(element.SourceRefs),
			States:      toCreativeElementStates(element.States),
		})
	}
	return out
}

func toCreativeElementStates(input []KeyElementStateInput) []creative.KeyElementStateInput {
	out := make([]creative.KeyElementStateInput, 0, len(input))
	for _, state := range input {
		out = append(out, creative.KeyElementStateInput{
			ClientKey:          state.ClientKey,
			Label:              state.Label,
			VisualDescription:  state.VisualDescription,
			ReferenceStatus:    state.ReferenceStatus,
			ReferenceNodeID:    state.ReferenceNodeID,
			ReferenceVersionID: state.ReferenceVersionID,
			IsDefault:          state.IsDefault,
			StateFacts:         toMemoryFacts(state.StateFacts),
			SourceRefs:         toCreativeElementSourceRefs(state.SourceRefs),
		})
	}
	return out
}

func toCreativeElementSourceRefs(input []ElementSourceRef) []creative.ElementSourceRef {
	out := make([]creative.ElementSourceRef, 0, len(input))
	for _, item := range input {
		out = append(out, creative.ElementSourceRef{Type: item.Type, ID: item.ID, Note: item.Note})
	}
	return out
}
