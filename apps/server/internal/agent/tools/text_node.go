package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var ErrAgentWorkspaceRequired = errors.New("agent workspace required")

type TextNodeStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	CreateTextMediaAsset(ctx context.Context, arg db.CreateTextMediaAssetParams) (db.MediaAsset, error)
	CreateAgentMediaNode(ctx context.Context, arg db.CreateAgentMediaNodeParams) (db.MediaNode, error)
}

type NodeBroadcaster interface {
	BroadcastAgentNodeCreated(workspaceID pgtype.UUID, node db.MediaNode)
}

type CreateAgentTextNodeTool struct {
	store       TextNodeStore
	broadcaster NodeBroadcaster
}

func NewCreateAgentTextNodeTool(store TextNodeStore, broadcaster NodeBroadcaster) CreateAgentTextNodeTool {
	return CreateAgentTextNodeTool{store: store, broadcaster: broadcaster}
}

func (t CreateAgentTextNodeTool) Definition() Definition {
	return Definition{
		Name:        "create_agent_text_node",
		Description: "Create an Agent-owned text source material node in the current Agent workspace. Use this only for durable briefs, scripts, notes, and user-approved creative direction.",
		Parameters: objectSchema(map[string]any{
			"title": map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
			"text":  map[string]any{"type": "string", "minLength": 1, "maxLength": 12000},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			WritesCanvas:    true,
			MaxCallsPerTurn: 10,
		},
		Visibility: VisibilitySpec{
			ShowCallMessage:   true,
			ShowResultMessage: true,
			UserLabel:         "创建文本素材",
		},
	}
}

func (t CreateAgentTextNodeTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.store == nil {
		return ExecuteOutput{}, errors.New("create_agent_text_node store is not configured")
	}
	args, err := textNodeArgs(input.Arguments)
	if err != nil {
		return ExecuteOutput{}, err
	}
	workspace, err := t.store.GetWorkspaceByID(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		return ExecuteOutput{}, ErrAgentWorkspaceRequired
	}
	asset, err := t.store.CreateTextMediaAsset(ctx, db.CreateTextMediaAssetParams{
		WorkspaceID: workspace.ID,
		TextContent: pgtype.Text{String: args.Text, Valid: true},
		SizeBytes:   pgtype.Int8{Int64: int64(len([]byte(args.Text))), Valid: true},
		Metadata:    jsonBytes(map[string]any{"title": args.Title, "source": "agent_tool"}),
	})
	if err != nil {
		return ExecuteOutput{}, err
	}
	node, err := t.store.CreateAgentMediaNode(ctx, db.CreateAgentMediaNodeParams{
		WorkspaceID: workspace.ID,
		NodeType:    db.NodeTypeText,
		Title:       args.Title,
		Prompt:      args.Text,
		AssetID:     asset.ID,
		CanvasX:     args.X,
		CanvasY:     args.Y,
		CanvasW:     280,
		CanvasH:     180,
	})
	if err != nil {
		return ExecuteOutput{}, err
	}
	if t.broadcaster != nil {
		t.broadcaster.BroadcastAgentNodeCreated(workspace.ID, node)
	}
	return ExecuteOutput{Result: map[string]any{
		"node_id":  uuidString(node.ID),
		"asset_id": uuidString(asset.ID),
		"title":    node.Title,
		"type":     string(node.NodeType),
	}}, nil
}

type createTextNodeArgs struct {
	Title string
	Text  string
	X     float32
	Y     float32
}

func textNodeArgs(raw map[string]any) (createTextNodeArgs, error) {
	title, _ := raw["title"].(string)
	text, _ := raw["text"].(string)
	title = strings.TrimSpace(title)
	text = strings.TrimSpace(text)
	if title == "" || len([]rune(title)) > 120 || text == "" || len([]rune(text)) > 12000 {
		return createTextNodeArgs{}, errors.New("invalid create_agent_text_node arguments")
	}
	args := createTextNodeArgs{Title: title, Text: text, X: 120, Y: 120}
	if placement, ok := raw["placement"].(map[string]any); ok {
		if x, ok := placement["x"].(float64); ok {
			args.X = float32(x)
		}
		if y, ok := placement["y"].(float64); ok {
			args.Y = float32(y)
		}
	}
	return args, nil
}

func jsonBytes(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
