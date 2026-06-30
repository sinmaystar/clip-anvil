package composer

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
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
	audioPlan, audioPlanOK, err := p.activeAudioPlan(ctx, runtime.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if audioPlanOK {
		audioAssets, err := p.audioPlanAssets(ctx, runtime.WorkspaceID, audioPlan)
		if err != nil {
			return nil, err
		}
		assets = append(assets, audioAssets...)
	}
	timelineSchema := map[string]any{
		"template_key": []string{"simple_concat", "concat_with_fades"},
		"segments":     "array of clip assets in final order",
		"audio_tracks": "array of voiceover and bgm tracks with role, asset_id, workspace_path, start_sec, duration_sec, volume, fade_in_sec, fade_out_sec and optional ducking",
		"output":       "final MP4 output settings; use audio_codec=aac when audio_tracks are present",
		"legacy_audio": "reserved for future original_audio handling",
	}
	result := map[string]any{
		"workspace_id":                 uuidString(runtime.WorkspaceID),
		"source_storyboard_node_id":    uuidString(sourceNodeID),
		"available_composition_assets": assets,
		"timeline_plan_schema":         timelineSchema,
	}
	if audioPlanOK {
		result["audio_plan"] = composerAudioPlanContext(audioPlan)
	}
	return result, nil
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
		node, role, ok, err := p.currentShotCompositionAsset(ctx, workspaceID, shot)
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
			"role":                role,
			"shot_id":             uuidString(shot.ID),
			"shot_ref":            defaultComposerRef(shot.SemanticKey, shot.ClientKey),
			"shot_title":          shot.Title,
			"node_id":             uuidString(node.ID),
			"node_ref":            composerNodeRef(node),
			"title":               node.Title,
			"artifact_kind":       defaultComposerRef(node.ArtifactKind, composerArtifactKind(node.Metadata)),
			"artifact_version_id": uuidString(version.ID),
			"asset_id":            uuidString(asset.ID),
			"source_url":          asset.StorageUrl.String,
			"mime_type":           asset.Mime,
			"file_name":           safeComposerAssetFileName(node.Title, asset.Mime),
		})
	}
	return out, nil
}

func (p ToolContextProvider) currentShotCompositionAsset(ctx context.Context, workspaceID pgtype.UUID, shot db.Shot) (db.MediaNode, string, bool, error) {
	nodes, err := p.store.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{WorkspaceID: workspaceID, ShotID: shot.ID})
	if err != nil {
		return db.MediaNode{}, "", false, err
	}
	if node, ok, err := p.currentShotAssetByKind(ctx, nodes, db.NodeTypeVideo, "shot_video"); err != nil || ok {
		return node, "clip", ok, err
	}
	node, ok, err := p.currentShotAssetByKind(ctx, nodes, db.NodeTypeImage, "preview_image")
	return node, "still", ok, err
}

func (p ToolContextProvider) currentShotAssetByKind(ctx context.Context, nodes []db.MediaNode, nodeType db.NodeType, artifactKind string) (db.MediaNode, bool, error) {
	for _, node := range nodes {
		if node.NodeType != nodeType || !node.CurrentVersionID.Valid {
			continue
		}
		if kind := defaultComposerRef(node.ArtifactKind, composerArtifactKind(node.Metadata)); kind != "" && kind != artifactKind {
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

func (p ToolContextProvider) activeAudioPlan(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, bool, error) {
	audioPlan, err := p.store.GetActiveAudioPlanByWorkspace(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.AudioPlan{}, false, nil
		}
		return db.AudioPlan{}, false, err
	}
	return audioPlan, true, nil
}

func (p ToolContextProvider) audioPlanAssets(ctx context.Context, workspaceID pgtype.UUID, audioPlan db.AudioPlan) ([]map[string]any, error) {
	if strings.TrimSpace(audioPlan.Status) != "approved" && strings.TrimSpace(audioPlan.Status) != "generating" && strings.TrimSpace(audioPlan.Status) != "voiceover_ready" && strings.TrimSpace(audioPlan.Status) != "composing" {
		return nil, nil
	}
	out := []map[string]any{}
	for _, item := range []struct {
		role   string
		nodeID pgtype.UUID
	}{
		{role: "voiceover", nodeID: audioPlan.VoiceoverNodeID},
		{role: "bgm", nodeID: audioPlan.BgmNodeID},
	} {
		if !item.nodeID.Valid {
			continue
		}
		asset, ok, err := p.audioNodeAsset(ctx, workspaceID, audioPlan, item.role, item.nodeID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, asset)
		}
	}
	return out, nil
}

func (p ToolContextProvider) audioNodeAsset(ctx context.Context, workspaceID pgtype.UUID, audioPlan db.AudioPlan, role string, nodeID pgtype.UUID) (map[string]any, bool, error) {
	node, err := p.store.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if node.WorkspaceID != workspaceID || node.NodeType != db.NodeTypeAudio || !node.CurrentVersionID.Valid {
		return nil, false, nil
	}
	version, err := p.store.GetArtifactVersionByID(ctx, node.CurrentVersionID)
	if err != nil {
		return nil, false, err
	}
	if version.Status != db.JobStatusSucceeded {
		return nil, false, nil
	}
	asset, err := p.store.GetMediaAssetByID(ctx, version.AssetID)
	if err != nil {
		return nil, false, err
	}
	return map[string]any{
		"role":                role,
		"audio_plan_id":       uuidString(audioPlan.ID),
		"audio_plan_ref":      defaultComposerRef(audioPlan.SemanticKey, "audio_plan.active"),
		"node_id":             uuidString(node.ID),
		"node_ref":            composerNodeRef(node),
		"title":               node.Title,
		"artifact_version_id": uuidString(version.ID),
		"asset_id":            uuidString(asset.ID),
		"source_url":          asset.StorageUrl.String,
		"mime_type":           asset.Mime,
		"file_name":           composerAudioAssetFileName(role, asset.Mime),
	}, true, nil
}

func composerAudioPlanContext(audioPlan db.AudioPlan) map[string]any {
	out := map[string]any{
		"id":               uuidString(audioPlan.ID),
		"status":           audioPlan.Status,
		"title":            audioPlan.Title,
		"semantic_key":     defaultComposerRef(audioPlan.SemanticKey, "audio_plan.active"),
		"display_name":     audioPlan.DisplayName,
		"language":         audioPlan.Language,
		"voiceover_script": audioPlan.VoiceoverScript,
		"voice_profile":    composerJSONValue(audioPlan.VoiceProfile),
		"bgm_plan":         composerJSONValue(audioPlan.BgmPlan),
		"cue_plan":         composerJSONValue(audioPlan.CuePlan),
	}
	if audioPlan.TargetDurationSec.Valid {
		out["target_duration_sec"] = audioPlan.TargetDurationSec.Float64
	}
	if audioPlan.VoiceoverNodeID.Valid {
		out["voiceover_node_id"] = uuidString(audioPlan.VoiceoverNodeID)
	}
	if audioPlan.BgmNodeID.Valid {
		out["bgm_node_id"] = uuidString(audioPlan.BgmNodeID)
	}
	if audioPlan.TimelinePlanID.Valid {
		out["timeline_plan_id"] = uuidString(audioPlan.TimelinePlanID)
	}
	return out
}

func composerJSONValue(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
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

func defaultComposerRef(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func composerAudioAssetFileName(role string, mime string) string {
	ext := ".bin"
	if strings.Contains(mime, "mpeg") || strings.Contains(mime, "mp3") {
		ext = ".mp3"
	} else if strings.Contains(mime, "wav") {
		ext = ".wav"
	} else if strings.Contains(mime, "ogg") {
		ext = ".ogg"
	}
	switch role {
	case "voiceover":
		return "voiceover" + ext
	case "bgm":
		return "bgm" + ext
	default:
		return "audio" + ext
	}
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
