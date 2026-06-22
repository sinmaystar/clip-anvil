package tools

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type WorkspaceContextStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
}

type ReadWorkspaceContextTool struct {
	store WorkspaceContextStore
}

type SourceMaterialRef struct {
	NodeID  string `json:"node_id"`
	AssetID string `json:"asset_id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
}

func NewReadWorkspaceContextTool(store WorkspaceContextStore) ReadWorkspaceContextTool {
	return ReadWorkspaceContextTool{store: store}
}

func (t ReadWorkspaceContextTool) Definition() Definition {
	return Definition{
		Name:        "read_workspace_context",
		Description: "Read the current ClipAnvil Agent workspace facts, including workspace metadata, source material refs, canvas summary, and task summary. This tool does not read message history.",
		Parameters: objectSchema(map[string]any{
			"include_assets":         map[string]any{"type": "boolean"},
			"include_canvas_summary": map[string]any{"type": "boolean"},
			"include_tasks":          map[string]any{"type": "boolean"},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			ReadOnly:        true,
			MaxCallsPerTurn: 10,
		},
		Visibility: VisibilitySpec{UserLabel: "读取项目上下文"},
	}
}

func (t ReadWorkspaceContextTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.store == nil {
		return ExecuteOutput{Result: map[string]any{}}, nil
	}
	workspace, err := t.store.GetWorkspaceByID(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	nodes, err := t.store.ListMediaNodesByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}

	refs := []SourceMaterialRef{}
	counts := map[string]int{}
	for _, node := range nodes {
		nodeType := string(node.NodeType)
		counts[nodeType]++
		if node.Source != "agent" || !node.AssetID.Valid {
			continue
		}
		refs = append(refs, SourceMaterialRef{
			NodeID:  uuidString(node.ID),
			AssetID: uuidString(node.AssetID),
			Type:    nodeType,
			Title:   node.Title,
		})
	}

	return ExecuteOutput{Result: map[string]any{
		"workspace": map[string]any{
			"id":    uuidString(workspace.ID),
			"title": workspace.Name,
			"mode":  string(workspace.Mode),
		},
		"source_material_refs":    refs,
		"source_material_summary": fmt.Sprintf("已有 %d 个 Agent 素材节点。", len(refs)),
		"canvas_summary":          fmt.Sprintf("当前画布包含 %d 个节点。", len(nodes)),
		"node_type_counts":        counts,
	}}, nil
}
