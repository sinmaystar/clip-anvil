package composer

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ToolContextProvider struct {
	store Store
}

func NewToolContextProvider(store Store) ToolContextProvider {
	return ToolContextProvider{store: store}
}

func (p ToolContextProvider) GetCompositionContext(ctx context.Context, runtime agenttools.NativeRuntimeContext, sourceNodeID pgtype.UUID) (map[string]any, error) {
	if p.store == nil {
		return nil, ErrInvalidConfig
	}
	if !sourceNodeID.Valid {
		sourceNodeID = runtime.ScopeID
	}
	assets, err := p.currentShotVideoAssets(ctx, runtime.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace_id":                 uuidString(runtime.WorkspaceID),
		"source_storyboard_node_id":    uuidString(sourceNodeID),
		"available_composition_assets": assets,
		"timeline_plan_schema": map[string]any{
			"template_key": []string{"simple_concat", "concat_with_fades"},
			"segments":     "array of clip assets in final order",
			"audio":        "reserved for bgm, voiceover, original_audio and ducking",
		},
	}, nil
}

func (p ToolContextProvider) currentShotVideoAssets(ctx context.Context, workspaceID pgtype.UUID) ([]map[string]any, error) {
	shots, err := p.store.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(shots, func(i, j int) bool {
		return shots[i].SortOrder < shots[j].SortOrder
	})
	out := []map[string]any{}
	for _, shot := range shots {
		node, ok, err := p.currentShotVideoWinner(ctx, workspaceID, shot)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		version, err := p.store.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return nil, err
		}
		asset, err := p.store.GetMediaAssetByID(ctx, version.AssetID)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"role":                "clip",
			"shot_id":             uuidString(shot.ID),
			"shot_title":          shot.Title,
			"node_id":             uuidString(node.ID),
			"node_ref":            composerNodeRef(node),
			"title":               node.Title,
			"artifact_version_id": uuidString(version.ID),
			"asset_id":            uuidString(asset.ID),
			"source_url":          asset.StorageUrl.String,
			"mime_type":           asset.Mime,
			"file_name":           safeComposerAssetFileName(node.Title, asset.Mime),
		})
	}
	return out, nil
}

func (p ToolContextProvider) currentShotVideoWinner(ctx context.Context, workspaceID pgtype.UUID, shot db.Shot) (db.MediaNode, bool, error) {
	nodes, err := p.store.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{WorkspaceID: workspaceID, ShotID: shot.ID})
	if err != nil {
		return db.MediaNode{}, false, err
	}
	for _, node := range nodes {
		if node.NodeType != db.NodeTypeVideo || !node.CurrentVersionID.Valid {
			continue
		}
		if kind := composerArtifactKind(node.Metadata); kind != "" && kind != "shot_video" {
			continue
		}
		version, err := p.store.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil || version.Status != db.JobStatusSucceeded {
			continue
		}
		return node, true, nil
	}
	return db.MediaNode{}, false, nil
}

func composerNodeRef(node db.MediaNode) string {
	if ref := strings.TrimSpace(node.SemanticKey); ref != "" {
		return ref
	}
	return uuidString(node.ID)
}

func composerArtifactKind(raw []byte) string {
	var metadata map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &metadata)
	}
	kind, _ := metadata["agent_artifact_kind"].(string)
	return strings.TrimSpace(kind)
}

func safeComposerAssetFileName(title string, mime string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = "clip"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\n", " ", "\t", " ")
	name = strings.TrimSpace(replacer.Replace(name))
	ext := path.Ext(name)
	if ext == "" {
		if strings.Contains(mime, "mp4") {
			ext = ".mp4"
		} else if strings.Contains(mime, "mpeg") {
			ext = ".mp3"
		} else {
			ext = ".bin"
		}
	}
	return strings.TrimSuffix(name, path.Ext(name)) + ext
}
