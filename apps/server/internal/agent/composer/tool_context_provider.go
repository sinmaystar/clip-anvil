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
	remotiontimeline "github.com/sinmaystar/clip-anvil/internal/remotiontimeline"
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
	if sourceAsset, ok, err := p.sourceUploadAsset(ctx, runtime.WorkspaceID, sourceNodeID); err != nil {
		return nil, err
	} else if ok {
		assets = prependUniqueComposerAsset(assets, sourceAsset)
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
		"template_key":              []string{"simple_concat", "concat_with_fades", remotiontimeline.TemplateKeyV1, "agent_remotion_code_v1"},
		"segments":                  "array of clip or still assets in final order; remotion_timeline_v1 segments may use asset type image or video",
		"audio_tracks":              "array of voiceover and bgm tracks with role, asset_id, workspace_path, start_sec, duration_sec, volume, fade_in_sec, fade_out_sec and optional ducking",
		"output":                    "final MP4 output settings; use audio_codec=aac when audio_tracks are present",
		"agent_remotion_code_v1":    "dynamic Agent-authored Remotion renderer; use create_remotion_renderer_attempt, validate_remotion_renderer_attempt, render_agent_remotion_renderer, then submit_composition_artifact",
		"dynamic_route_persistence": "sandbox files are editable working state; renderer artifact and attempt snapshots are durable DB facts",
	}
	result := map[string]any{
		"workspace_id":                 uuidString(runtime.WorkspaceID),
		"source_storyboard_node_id":    uuidString(sourceNodeID),
		"available_composition_assets": assets,
		"timeline_plan_schema":         timelineSchema,
		"remotion_timeline_schema":     remotionTimelineSchemaContext(),
	}
	if audioPlanOK {
		result["audio_plan"] = composerAudioPlanContext(audioPlan)
	}
	return result, nil
}

func (p ToolContextProvider) sourceUploadAsset(ctx context.Context, workspaceID pgtype.UUID, sourceNodeID pgtype.UUID) (map[string]any, bool, error) {
	if !sourceNodeID.Valid {
		return nil, false, nil
	}
	node, err := p.store.GetMediaNodeByID(ctx, sourceNodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if node.WorkspaceID != workspaceID || !node.AssetID.Valid {
		return nil, false, nil
	}
	asset, err := p.store.GetMediaAssetByID(ctx, node.AssetID)
	if err != nil {
		return nil, false, err
	}
	if asset.WorkspaceID != workspaceID {
		return nil, false, nil
	}
	role := "still"
	if node.NodeType == db.NodeTypeVideo {
		role = "clip"
	}
	return map[string]any{
		"role":       role,
		"node_id":    uuidString(node.ID),
		"node_ref":   defaultComposerRef(node.SemanticKey, uuidString(node.ID)),
		"title":      node.Title,
		"asset_id":   uuidString(asset.ID),
		"source_url": asset.StorageUrl.String,
		"mime_type":  asset.Mime,
		"file_name":  safeComposerAssetFileName(firstNonEmpty(node.Title, firstNonEmpty(composerAssetFilename(asset.Metadata), "source-upload")), asset.Mime),
		"metadata":   composerMediaAssetMetadata(asset.Metadata),
	}, true, nil
}

func prependUniqueComposerAsset(assets []map[string]any, source map[string]any) []map[string]any {
	assetID := strings.TrimSpace(composerString(source["asset_id"]))
	if assetID == "" {
		return assets
	}
	for _, asset := range assets {
		if strings.TrimSpace(composerString(asset["asset_id"])) == assetID {
			return assets
		}
	}
	return append([]map[string]any{source}, assets...)
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
		assetContext := map[string]any{
			"role":                role,
			"shot_id":             uuidString(shot.ID),
			"shot_ref":            defaultComposerRef(shot.SemanticKey, shot.ClientKey),
			"shot_title":          shot.Title,
			"sort_order":          shot.SortOrder,
			"narrative_purpose":   shot.NarrativePurpose,
			"visual_intent":       shot.VisualIntent,
			"action_text":         shot.ActionText,
			"camera_intent":       shot.CameraIntent,
			"narration":           shot.Narration,
			"node_id":             uuidString(node.ID),
			"node_ref":            composerNodeRef(node),
			"title":               node.Title,
			"artifact_kind":       defaultComposerRef(node.ArtifactKind, composerArtifactKind(node.Metadata)),
			"artifact_version_id": uuidString(version.ID),
			"asset_id":            uuidString(asset.ID),
			"source_url":          asset.StorageUrl.String,
			"mime_type":           asset.Mime,
			"file_name":           safeComposerAssetFileName(node.Title, asset.Mime),
		}
		if shot.DurationSec.Valid {
			assetContext["duration_sec"] = shot.DurationSec.Float64
		}
		out = append(out, assetContext)
	}
	return out, nil
}

func remotionTimelineSchemaContext() map[string]any {
	return map[string]any{
		"schema":       remotiontimeline.SchemaV1,
		"composition":  remotiontimeline.CompositionMarketingTimeline,
		"template_key": remotiontimeline.TemplateKeyV1,
		"asset_types":  []string{"image", "video"},
		"layouts": []string{
			"hero_packshot",
			"detail_focus",
			"benefit_card",
			"split_compare",
			"scenario_card",
			"open_storage",
			"cta_endcard",
		},
		"motions": []string{
			"push_in",
			"pull_out",
			"pan_left",
			"pan_right",
			"float_parallax",
			"spotlight_reveal",
			"kinetic_text",
			"cta_pop",
		},
		"transitions":       []string{"cut", "crossfade", "slide", "wipe", "zoom_blur"},
		"caption_source":    "audio_plan.cue_plan.caption, voiceover_alignment, tts_alignment, or manual_caption only; never narrative_purpose, visual_intent, action_text, camera_intent, or internal director notes",
		"caption_lane":      "single Composer-owned subtitle_bottom lane; text layers must stay outside the bottom 18 percent safe area",
		"asset_matching":    "match cue shot_ref and visual_focus to same-shot still/clip; wheel cues require wheel/detail assets; storage cues require open/interior/storage assets; clip assets render as video segments and still assets render as image segments",
		"repetition_limits": "avoid using the same visual asset for more than half of segments and avoid the same layout more than two segments in a row",
		"route_policy":      "no-seedance uses Seedream still image segments only; mixed-cost may include limited existing Seedance clip assets for hero or complex-motion cues; premium may include more clips, but remotion_timeline_v1 remains the final packaging engine",
	}
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
	seenRoles := map[string]bool{}
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
			seenRoles[item.role] = true
		}
	}
	if seenRoles["voiceover"] && seenRoles["bgm"] {
		return out, nil
	}
	nodes, err := p.store.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, role := range []string{"voiceover", "bgm"} {
		if seenRoles[role] {
			continue
		}
		node, ok := composerFallbackAudioNode(nodes, role)
		if !ok {
			continue
		}
		asset, ok, err := p.audioNodeAsset(ctx, workspaceID, audioPlan, role, node.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, asset)
		}
	}
	return out, nil
}

func composerFallbackAudioNode(nodes []db.MediaNode, role string) (db.MediaNode, bool) {
	artifactKind := role + "_audio"
	prefix := "audio_plan.active." + artifactKind
	for i := len(nodes) - 1; i >= 0; i-- {
		node := nodes[i]
		if node.NodeType != db.NodeTypeAudio || !node.CurrentVersionID.Valid {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(node.SemanticKey), prefix) {
			return node, true
		}
		if composerArtifactKind(node.Metadata) == artifactKind {
			return node, true
		}
	}
	return db.MediaNode{}, false
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
		"metadata":            composerJSONValue(asset.Metadata),
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

func composerAssetFilename(raw []byte) string {
	metadata := composerMediaAssetMetadata(raw)
	return strings.TrimSpace(composerString(metadata["filename"]))
}

func composerMediaAssetMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
