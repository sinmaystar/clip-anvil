package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ToolObjectRef struct {
	Type string `json:"type" jsonschema:"required,enum=creative_brief,enum=project_memory,enum=key_element,enum=key_element_state,enum=scene,enum=shot,enum=shot_dependency,enum=render_plan,enum=media_node,enum=artifact_version,enum=review_record,enum=artifact_issue" jsonschema_description:"对象类型。优先使用 read_project_context 返回的 semantic_key，不要填写 UUID。"`
	Key  string `json:"key" jsonschema:"required" jsonschema_description:"对象稳定语义键，例如 shot_03、scene_airport_departure、element_luggage.state_silver_reference。不要填写 UUID。"`
}

type ToolArtifactRef struct {
	Key          string        `json:"key" jsonschema_description:"artifact_version 的完整语义键，例如 shot_03.preview_image.rp1.output.v1。若填写 key，scope/artifact_kind/selector 可为空。"`
	Scope        ToolObjectRef `json:"scope" jsonschema_description:"按作用域选择 artifact，例如 type=shot,key=shot_03。"`
	ArtifactKind string        `json:"artifact_kind" jsonschema:"enum=reference_image,enum=preview_image,enum=shot_video,enum=final_video" jsonschema_description:"要选择的产物类型。"`
	Selector     string        `json:"selector" jsonschema:"enum=current,enum=latest,enum=winner" jsonschema_description:"选择器。current/winner 表示当前选中版本；latest 表示最新版本。默认 current。"`
}

type AgentObjectRefResolver struct {
	queries interface {
		ListAgentObjectsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentObjectIndex, error)
	}
}

func NewAgentObjectRefResolver(queries interface {
	ListAgentObjectsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentObjectIndex, error)
}) AgentObjectRefResolver {
	return AgentObjectRefResolver{queries: queries}
}

func (r AgentObjectRefResolver) ResolveObjectRef(ctx context.Context, workspaceID pgtype.UUID, ref ToolObjectRef) (pgtype.UUID, bool, error) {
	if r.queries == nil {
		return pgtype.UUID{}, false, nil
	}
	rows, err := r.queries.ListAgentObjectsByWorkspace(ctx, workspaceID)
	if err != nil {
		return pgtype.UUID{}, false, err
	}
	objectType := strings.TrimSpace(ref.Type)
	key := strings.TrimSpace(ref.Key)
	for _, row := range rows {
		if row.ObjectType == objectType && row.SemanticKey == key {
			return row.ObjectID, true, nil
		}
	}
	return pgtype.UUID{}, false, nil
}

func validateObjectRef(ref ToolObjectRef, field string) error {
	if strings.TrimSpace(ref.Type) == "" {
		return fmt.Errorf("%s.type 必填", field)
	}
	if strings.TrimSpace(ref.Key) == "" {
		return fmt.Errorf("%s.key 必填，请使用 read_project_context 返回的 semantic_key，不要编造 UUID", field)
	}
	return nil
}

func validateArtifactRef(ref ToolArtifactRef, field string) error {
	if strings.TrimSpace(ref.Key) != "" {
		return nil
	}
	if err := validateObjectRef(ref.Scope, field+".scope"); err != nil {
		return err
	}
	if strings.TrimSpace(ref.ArtifactKind) == "" {
		return fmt.Errorf("%s.artifact_kind 必填", field)
	}
	selector := strings.TrimSpace(ref.Selector)
	if selector != "" && selector != "current" && selector != "latest" && selector != "winner" {
		return fmt.Errorf("%s.selector 只能是 current、latest 或 winner", field)
	}
	return nil
}
