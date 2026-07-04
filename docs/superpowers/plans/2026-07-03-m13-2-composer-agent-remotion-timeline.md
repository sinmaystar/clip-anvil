# M13.2 Composer Agent Remotion Timeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the real Composer Agent create `remotion_timeline_v1` plans from Storyboard, AudioPlan cue_plan, Seedream stills, voiceover, and BGM in no-Seedance Agent workflows.

**Architecture:** M13.1 already added the renderer path. M13.2 makes Agent routing use it: enrich `get_composition_context`, teach Composer prompts/skills and deterministic fallback to emit `RemotionTimelinePlan`, and update Producer no-Seedance guidance so the low-cost route is Seedream stills plus Remotion final timeline instead of mandatory per-shot `motion_shot_video`.

**Tech Stack:** Go 1.26, pgx/sqlc models, Eino tool calling, embedded Agent skills, existing Composer native tools, Remotion timeline schema from `apps/server/internal/remotiontimeline`.

---

## File Structure

- Modify `apps/server/internal/agent/composer/tool_context_provider.go`: expose still assets, shot metadata, AudioPlan cue_plan, and `remotion_timeline_schema`.
- Modify `apps/server/internal/agent/composer/tool_context_provider_test.go`: prove context contains first-class still assets and Remotion schema.
- Modify `apps/server/internal/agent/composer/model_responder.go`: allow deterministic template path for `remotion_timeline_v1` and generate schema-valid Remotion timeline plans from cue_plan.
- Modify `apps/server/internal/agent/composer/model_responder_test.go`: prove deterministic Remotion plan uses cue order, captions, real staged paths, audio tracks, and avoids director-note captions.
- Modify `apps/server/internal/agent/composer/system_prompt.go`: describe Remotion timeline route, still-image priority, cue_plan as sync contract, and forbidden subtitle sources.
- Add `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`: Composer skill for Remotion final timeline planning.
- Modify `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`: make `remotion_timeline_v1` a first-class timeline template.
- Modify `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md` and `motion-shot-producer/SKILL.md`: route no-Seedance final videos to `remotion_timeline_v1` with Seedream stills and Volcengine audio.
- Modify `apps/server/internal/agent/skills/registry_test.go`: update expected skill list and required phrase tests.
- Modify `apps/server/internal/agent/producer/system_prompt.go`: replace old no-Seedance guidance that required every shot video with the new stills plus Remotion final timeline policy.
- Modify `apps/server/internal/agent/producer/model_responder_test.go`: update prompt assertions.
- Add `scripts/smoke-m13-2-agent-remotion-route.sql`: DB acceptance query template for E2E validation.
- Modify `docs/milestones/m13-remotion-timeline-composer.md`: mark M13.2 status and record verification when complete.

## Task 1: Enrich Composer Composition Context

**Files:**
- Modify: `apps/server/internal/agent/composer/tool_context_provider.go`
- Modify: `apps/server/internal/agent/composer/tool_context_provider_test.go`

- [ ] **Step 1: Write failing context test**

Add this test to `tool_context_provider_test.go`:

```go
func TestCompositionContextIncludesRemotionTimelineInputs(t *testing.T) {
	workspaceID := uuidWithByte(1)
	wheelsShotID := uuidWithByte(21)
	storageShotID := uuidWithByte(22)
	wheelsVersionID := uuidWithByte(31)
	storageVersionID := uuidWithByte(32)
	wheelsAssetID := uuidWithByte(41)
	storageAssetID := uuidWithByte(42)
	audioPlanID := uuidWithByte(50)

	cuePlan := mustComposerJSON(t, []map[string]any{
		{"shot_ref": "shot_wheels", "start_sec": 0, "end_sec": 5, "text": "顺滑万向轮，转弯不费力。", "caption": "顺滑万向轮"},
		{"shot_ref": "shot_storage", "start_sec": 5, "end_sec": 10, "text": "打开就是分区收纳。", "caption": "分区收纳"},
	})
	store := &fakeComposerStore{
		audioPlan: &db.AudioPlan{
			ID:                audioPlanID,
			WorkspaceID:       workspaceID,
			Status:            "approved",
			Title:             "悦行行李箱音频方案",
			TargetDurationSec: float8Value(30),
			CuePlan:           cuePlan,
			SemanticKey:       "audio_plan.active",
		},
		shots: []db.Shot{
			{
				ID:               wheelsShotID,
				WorkspaceID:      workspaceID,
				ClientKey:        "shot_wheels",
				SemanticKey:      "shot_wheels",
				SortOrder:        2,
				Title:            "万向轮特写",
				DurationSec:      float8Value(5),
				NarrativePurpose: "证明短途出行好推",
				VisualIntent:     "轮组近景，展示顺滑转向",
				ActionText:       "行李箱轻推转弯",
				Narration:        "顺滑万向轮，转弯不费力。",
			},
			{
				ID:               storageShotID,
				WorkspaceID:      workspaceID,
				ClientKey:        "shot_storage",
				SemanticKey:      "shot_storage",
				SortOrder:        3,
				Title:            "打开收纳",
				DurationSec:      float8Value(5),
				NarrativePurpose: "展示容量和分区",
				VisualIntent:     "打开箱体内景，衣物和电脑分区",
				ActionText:       "箱体打开露出分层空间",
				Narration:        "打开就是分区收纳。",
			},
		},
		nodesByShot: map[pgtype.UUID][]db.MediaNode{
			wheelsShotID: {{
				ID:               uuidWithByte(61),
				WorkspaceID:      workspaceID,
				ShotID:           wheelsShotID,
				NodeType:         db.NodeTypeImage,
				Title:            "wheel detail image",
				CurrentVersionID: wheelsVersionID,
				ArtifactKind:     "preview_image",
				SemanticKey:      "shot_wheels.preview_image.r1.node",
			}},
			storageShotID: {{
				ID:               uuidWithByte(62),
				WorkspaceID:      workspaceID,
				ShotID:           storageShotID,
				NodeType:         db.NodeTypeImage,
				Title:            "storage interior image",
				CurrentVersionID: storageVersionID,
				ArtifactKind:     "preview_image",
				SemanticKey:      "shot_storage.preview_image.r1.node",
			}},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			wheelsVersionID:  {ID: wheelsVersionID, AssetID: wheelsAssetID, Status: db.JobStatusSucceeded},
			storageVersionID: {ID: storageVersionID, AssetID: storageAssetID, Status: db.JobStatusSucceeded},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			wheelsAssetID:  {ID: wheelsAssetID, WorkspaceID: workspaceID, Mime: "image/png", StorageUrl: textValue("workspace/wheels.png")},
			storageAssetID: {ID: storageAssetID, WorkspaceID: workspaceID, Mime: "image/png", StorageUrl: textValue("workspace/storage.png")},
		},
	}

	ctx, err := NewToolContextProvider(store).GetCompositionContext(context.Background(), agenttools.NativeRuntimeContext{WorkspaceID: workspaceID}, pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	assets, _ := ctx["available_composition_assets"].([]map[string]any)
	if len(assets) != 2 {
		t.Fatalf("assets = %#v", assets)
	}
	wheels, ok := composerContextAssetByNodeRef(assets, "shot_wheels.preview_image.r1.node")
	if !ok {
		t.Fatalf("wheel still asset missing: %#v", assets)
	}
	for key, want := range map[string]any{
		"role":              "still",
		"shot_ref":          "shot_wheels",
		"shot_title":        "万向轮特写",
		"duration_sec":      float64(5),
		"narrative_purpose": "证明短途出行好推",
		"visual_intent":     "轮组近景，展示顺滑转向",
		"sort_order":        int32(2),
	} {
		if wheels[key] != want {
			t.Fatalf("wheels[%s] = %#v, want %#v; asset=%#v", key, wheels[key], want, wheels)
		}
	}
	remotionSchema, ok := ctx["remotion_timeline_schema"].(map[string]any)
	if !ok {
		t.Fatalf("remotion_timeline_schema missing: %#v", ctx)
	}
	if remotionSchema["schema"] != "clipanvil.remotion_timeline.v1" || remotionSchema["composition"] != "MarketingTimeline" {
		t.Fatalf("remotion schema = %#v", remotionSchema)
	}
	layouts, _ := remotionSchema["layouts"].([]string)
	if !hasComposerString(layouts, "detail_focus") || !hasComposerString(layouts, "open_storage") {
		t.Fatalf("remotion layouts = %#v", layouts)
	}
}
```

Also add helper:

```go
func hasComposerString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test and confirm failure**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run TestCompositionContextIncludesRemotionTimelineInputs -count=1
```

Expected: FAIL because context does not yet include Remotion schema and full shot metadata.

- [ ] **Step 3: Implement context enrichment**

In `tool_context_provider.go`, import the Remotion timeline package:

```go
remotiontimeline "github.com/sinmaystar/clip-anvil/internal/remotiontimeline"
```

Add `remotion_timeline_v1` to `timeline_plan_schema.template_key` and add a top-level `remotion_timeline_schema`:

```go
timelineSchema := map[string]any{
	"template_key": []string{"simple_concat", "concat_with_fades", remotiontimeline.TemplateKeyV1},
	"segments":     "array of clip or still assets in final order",
	"audio_tracks": "array of voiceover and bgm tracks with role, asset_id, workspace_path, start_sec, duration_sec, volume, fade_in_sec, fade_out_sec and optional ducking",
	"output":       "final MP4 output settings; use audio_codec=aac when audio_tracks are present",
}
result := map[string]any{
	"workspace_id":                 uuidString(runtime.WorkspaceID),
	"source_storyboard_node_id":    uuidString(sourceNodeID),
	"available_composition_assets": assets,
	"timeline_plan_schema":         timelineSchema,
	"remotion_timeline_schema":     remotionTimelineSchemaContext(),
}
```

Extend each shot asset:

```go
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
```

Add schema helper:

```go
func remotionTimelineSchemaContext() map[string]any {
	return map[string]any{
		"schema":      remotiontimeline.SchemaV1,
		"composition": remotiontimeline.CompositionMarketingTimeline,
		"template_key": remotiontimeline.TemplateKeyV1,
		"layouts": []string{
			"hero_packshot",
			"detail_focus",
			"benefit_card",
			"open_storage",
			"cta_endcard",
		},
		"motions": []string{
			"push_in",
			"pull_out",
			"pan_left",
			"pan_right",
			"float_parallax",
			"cta_pop",
		},
		"transitions": []string{"fade", "crossfade", "slide_up", "wipe"},
		"caption_source": "audio_plan.cue_plan.caption or voiceover alignment only; never narrative_purpose, visual_intent, action_text, camera_intent, or internal director notes",
	}
}
```

- [ ] **Step 4: Run context tests**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run 'CompositionContext|RemotionTimelineInputs' -count=1
```

Expected: PASS.

## Task 2: Deterministic Remotion Timeline Plan

**Files:**
- Modify: `apps/server/internal/agent/composer/model_responder.go`
- Modify: `apps/server/internal/agent/composer/model_responder_test.go`

- [ ] **Step 1: Write failing deterministic plan test**

Add this test:

```go
func TestDeterministicComposerRemotionTimelinePlanUsesCuePlanAndStills(t *testing.T) {
	compositionContext := map[string]any{
		"audio_plan": map[string]any{
			"target_duration_sec": float64(30),
			"cue_plan": []any{
				map[string]any{"shot_ref": "shot_wheels", "start_sec": float64(0), "end_sec": float64(15), "text": "顺滑万向轮，转弯不费力。", "caption": "顺滑万向轮"},
				map[string]any{"shot_ref": "shot_storage", "start_sec": float64(15), "end_sec": float64(30), "text": "打开就是分区收纳。", "caption": "分区收纳"},
			},
		},
		"available_composition_assets": []any{
			map[string]any{"role": "still", "shot_ref": "shot_storage", "shot_title": "打开收纳", "asset_id": "asset-storage", "mime_type": "image/png", "visual_intent": "打开箱体内景"},
			map[string]any{"role": "still", "shot_ref": "shot_wheels", "shot_title": "万向轮特写", "asset_id": "asset-wheels", "mime_type": "image/png", "visual_intent": "轮组近景"},
			map[string]any{"role": "voiceover", "asset_id": "asset-voice", "mime_type": "audio/mpeg", "metadata": map[string]any{"duration_sec": float64(30)}},
			map[string]any{"role": "bgm", "asset_id": "asset-bgm", "mime_type": "audio/mpeg"},
		},
	}
	staged := map[string]any{
		"files": []any{
			map[string]any{"asset_id": "asset-storage", "workspace_path": "/workspace/input/storage.png"},
			map[string]any{"asset_id": "asset-wheels", "workspace_path": "/workspace/input/wheels.png"},
			map[string]any{"asset_id": "asset-voice", "workspace_path": "/workspace/input/voiceover.mp3"},
			map[string]any{"asset_id": "asset-bgm", "workspace_path": "/workspace/input/bgm.mp3"},
		},
	}
	context := Context{SameTurnMessages: []ComposerSameTurnMessage{
		composerToolResultMessage(t, "get_composition_context", compositionContext),
		composerToolResultMessage(t, "stage_media_inputs", staged),
	}}

	plan, err := deterministicComposerTimelinePlan(context, "remotion_timeline_v1")
	if err != nil {
		t.Fatal(err)
	}
	if plan["schema"] != "clipanvil.remotion_timeline.v1" || plan["composition"] != "MarketingTimeline" {
		t.Fatalf("remotion plan header = %#v", plan)
	}
	output, _ := plan["output"].(map[string]any)
	if output["width"] != 1080 || output["height"] != 1920 || output["duration_sec"] != float64(30) {
		t.Fatalf("output = %#v", output)
	}
	segments, ok := plan["segments"].([]any)
	if !ok || len(segments) != 2 {
		t.Fatalf("segments = %#v", plan["segments"])
	}
	first := segments[0].(map[string]any)
	second := segments[1].(map[string]any)
	if first["id"] != "shot_wheels" || first["start_sec"] != float64(0) || first["end_sec"] != float64(15) {
		t.Fatalf("first segment = %#v", first)
	}
	if second["id"] != "shot_storage" || second["start_sec"] != float64(15) || second["end_sec"] != float64(30) {
		t.Fatalf("second segment = %#v", second)
	}
	firstAssets := first["assets"].([]any)
	if firstAssets[0].(map[string]any)["workspace_path"] != "/workspace/input/wheels.png" {
		t.Fatalf("wheel cue not matched to wheel still: %#v", firstAssets)
	}
	firstCaptions := first["captions"].([]any)
	if firstCaptions[0].(map[string]any)["text"] != "顺滑万向轮" {
		t.Fatalf("caption should come from cue caption: %#v", firstCaptions)
	}
	if strings.Contains(string(mustJSON(plan)), "轮组近景") || strings.Contains(string(mustJSON(plan)), "打开箱体内景") {
		t.Fatalf("internal visual_intent leaked into captions or text layers: %#v", plan)
	}
	audioTracks, ok := plan["audio_tracks"].([]any)
	if !ok || len(audioTracks) != 2 {
		t.Fatalf("audio_tracks = %#v", plan["audio_tracks"])
	}
}
```

- [ ] **Step 2: Run test and confirm failure**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run TestDeterministicComposerRemotionTimelinePlanUsesCuePlanAndStills -count=1
```

Expected: FAIL because deterministic path currently ignores `remotion_timeline_v1`.

- [ ] **Step 3: Allow deterministic path for Remotion template**

In `deterministicTemplateComposerTurn`, change the guard:

```go
if templateKey != "simple_concat" && templateKey != "concat_with_fades" && templateKey != "remotion_timeline_v1" {
	return ComposerTurnOutput{}, false, nil
}
```

- [ ] **Step 4: Split legacy and Remotion plan builders**

In `deterministicComposerTimelinePlan`, branch early:

```go
if templateKey == "remotion_timeline_v1" {
	return deterministicComposerRemotionTimelinePlan(composerContext)
}
```

Add `deterministicComposerRemotionTimelinePlan`:

```go
func deterministicComposerRemotionTimelinePlan(composerContext Context) (map[string]any, error) {
	compositionContext, err := composerToolResultMap(composerContext, "get_composition_context")
	if err != nil {
		return nil, err
	}
	staged, err := composerToolResultMap(composerContext, "stage_media_inputs")
	if err != nil {
		return nil, err
	}
	pathsByAsset := composerStagedPaths(staged)
	rawAssets, _ := compositionContext["available_composition_assets"].([]any)
	cues := composerAudioCues(compositionContext)
	if len(cues) == 0 {
		return nil, fmt.Errorf("remotion timeline requires audio_plan.cue_plan")
	}
	targetDuration := composerBestAudioTargetDuration(compositionContext, rawAssets)
	if targetDuration <= 0 {
		targetDuration = composerAudioTargetDuration(compositionContext)
	}
	scale := composerCueDurationScale(cues, targetDuration)
	assetsByShot := composerVisualAssetsByShot(rawAssets, pathsByAsset)
	segments := []any{}
	for _, cue := range cues {
		asset := assetsByShot[cue.ShotRef]
		if asset == nil {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		start := cue.StartSec * scale
		end := cue.EndSec * scale
		if end <= start {
			continue
		}
		segments = append(segments, map[string]any{
			"id":        cue.ShotRef,
			"start_sec": start,
			"end_sec":   end,
			"layout":    composerRemotionLayoutForAsset(asset, cue),
			"assets": []any{
				map[string]any{
					"id":             assetID,
					"role":           "primary",
					"type":           composerRemotionAssetType(asset),
					"workspace_path": pathsByAsset[assetID],
					"node_ref":       strings.TrimSpace(composerString(asset["node_ref"])),
				},
			},
			"motion": map[string]any{"preset": composerRemotionMotionForAsset(asset, cue), "intensity": 0.55},
			"captions": []any{
				map[string]any{"text": firstNonEmpty(cue.Caption, cue.Text), "start_sec": start, "end_sec": end},
			},
		})
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("remotion timeline has no cue-matched staged still or clip assets")
	}
	audioTracks := deterministicComposerRemotionAudioTracks(rawAssets, pathsByAsset, targetDuration)
	return map[string]any{
		"schema":       "clipanvil.remotion_timeline.v1",
		"composition":  "MarketingTimeline",
		"template_key": "remotion_timeline_v1",
		"output": map[string]any{
			"width":        1080,
			"height":       1920,
			"fps":          30,
			"duration_sec": targetDuration,
			"codec":        "h264",
			"audio_codec":  "aac",
		},
		"segments":     segments,
		"audio_tracks": audioTracks,
		"captions":     map[string]any{"source": "audio_plan.cue_plan"},
		"theme":        map[string]any{"background": "#0f172a", "accent": "#facc15"},
	}, nil
}
```

Add helper functions with deterministic defaults:

```go
func composerVisualAssetsByShot(rawAssets []any, pathsByAsset map[string]string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(composerString(asset["role"]))
		if role != "still" && role != "clip" {
			continue
		}
		shotRef := strings.TrimSpace(composerString(asset["shot_ref"]))
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		if shotRef == "" || pathsByAsset[assetID] == "" {
			continue
		}
		if out[shotRef] == nil || role == "still" {
			out[shotRef] = asset
		}
	}
	return out
}

func composerRemotionAssetType(asset map[string]any) string {
	role := strings.TrimSpace(composerString(asset["role"]))
	if role == "clip" {
		return "video"
	}
	return "image"
}

func composerRemotionLayoutForAsset(asset map[string]any, cue composerAudioCue) string {
	text := strings.ToLower(strings.Join([]string{
		composerString(asset["shot_ref"]),
		composerString(asset["shot_title"]),
		composerString(asset["visual_intent"]),
		cue.Text,
		cue.Caption,
	}, " "))
	switch {
	case strings.Contains(text, "wheel") || strings.Contains(text, "轮"):
		return "detail_focus"
	case strings.Contains(text, "storage") || strings.Contains(text, "收纳") || strings.Contains(text, "内景"):
		return "open_storage"
	case strings.Contains(text, "cta") || strings.Contains(text, "出发"):
		return "cta_endcard"
	default:
		return "hero_packshot"
	}
}

func composerRemotionMotionForAsset(asset map[string]any, cue composerAudioCue) string {
	layout := composerRemotionLayoutForAsset(asset, cue)
	switch layout {
	case "detail_focus":
		return "push_in"
	case "open_storage":
		return "pull_out"
	case "cta_endcard":
		return "cta_pop"
	default:
		return "float_parallax"
	}
}
```

Add audio helper:

```go
func deterministicComposerRemotionAudioTracks(rawAssets []any, pathsByAsset map[string]string, duration float64) []any {
	tracks := []any{}
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(composerString(asset["role"]))
		if role != "voiceover" && role != "bgm" {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		path := pathsByAsset[assetID]
		if path == "" {
			continue
		}
		track := map[string]any{
			"id":             role + "-main",
			"role":           role,
			"asset_id":       assetID,
			"workspace_path": path,
			"start_sec":      0,
			"duration_sec":   duration,
			"volume":         1,
			"fade_in_sec":    0.05,
			"fade_out_sec":   0.1,
		}
		if role == "bgm" {
			track["volume"] = 0.28
			track["fade_in_sec"] = 0.5
			track["fade_out_sec"] = 1.2
		}
		tracks = append(tracks, track)
	}
	return tracks
}
```

- [ ] **Step 5: Run deterministic composer tests**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run 'DeterministicComposer.*Remotion|ComposerResponderUsesDeterministicTemplatePath' -count=1
```

Expected: PASS.

## Task 3: Prompt And Skill Route Guidance

**Files:**
- Modify: `apps/server/internal/agent/composer/system_prompt.go`
- Add: `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/motion-shot-producer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/registry_test.go`
- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`

- [ ] **Step 1: Write failing skill registry tests**

Update `TestM82DefaultRegistryContainsCommerceSkillPack` so Composer expects:

```go
RoleComposer: {
	"composer-blocker-escalation",
	"composer-timeline-director",
	"ffmpeg-audio-mix-composer",
	"platform-export-composer",
	"remotion-timeline-composer",
},
```

Add:

```go
func TestRemotionTimelineComposerSkillGuidesCueAlignedStillTimeline(t *testing.T) {
	loaded, err := DefaultRegistry().Load("remotion-timeline-composer", RoleComposer, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"remotion_timeline_v1",
		"still images are first-class visual assets",
		"AudioPlan cue_plan is the primary timing contract",
		"Do not use narrative_purpose, visual_intent, action_text, or camera_intent as captions",
		"match wheel cues to wheel/detail assets and storage cues to open-interior assets",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("remotion-timeline-composer missing %q\n%s", needle, loaded.Content)
		}
	}
}
```

Add prompt assertions in `producer/model_responder_test.go`:

```go
func TestProducerPromptPrefersRemotionTimelineForNoSeedanceLowCostRoute(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, needle := range []string{
		"no-Seedance low-cost final route should prefer Seedream stills plus remotion_timeline_v1 final Composer",
		"do not require every shot to become motion_shot_video",
		"dispatch_composer with template_key=remotion_timeline_v1",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("producer prompt missing %q", needle)
		}
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills ./internal/agent/producer -run 'RemotionTimelineComposer|PrefersRemotionTimeline|DefaultRegistryContainsCommerceSkillPack' -count=1
```

Expected: FAIL because the skill and prompt guidance are not present yet.

- [ ] **Step 3: Add Remotion Composer skill**

Create `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`:

```markdown
---
name: remotion-timeline-composer
description: Use when Composer builds a low-cost final marketing video with remotion_timeline_v1 from Seedream stills, shot metadata, AudioPlan cue_plan, voiceover, BGM, captions, and final layout/motion choices.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video, remotion_timeline, commerce_ad]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, render_timeline_template, submit_composition_artifact]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Remotion Timeline Composer

## Use When

Load this skill when `template_key=remotion_timeline_v1`, when Producer requests a low-cost no-Seedance final video, or when the final video should be assembled from still images, voiceover, BGM, captions, and Remotion layout/motion.

## Do

- Use `remotion_timeline_v1`.
- Treat still images as first-class visual assets, not as emergency fallback.
- Treat AudioPlan cue_plan as the primary timing contract.
- Match every cue `shot_ref` to a staged still or clip with the same `shot_ref`.
- Use cue `caption` or voiceover alignment for final subtitles.
- Match wheel cues to wheel/detail assets and storage cues to open-interior assets.
- Keep one caption layer inside the final Remotion timeline.
- Use generated voiceover as the primary audio track and BGM as supporting audio.

## Do Not

- Do not require every shot to have `motion_shot_video`.
- Do not use narrative_purpose, visual_intent, action_text, or camera_intent as captions.
- Do not reuse a generic full-product still for every benefit when shot-specific stills are available.
- Do not create raw Remotion code.
- Do not submit a silent final video when approved audio assets exist.

## Tool Protocol

1. Call `get_composition_context`.
2. Verify `audio_plan.cue_plan`, voiceover, BGM, and cue-matched still or clip assets.
3. Call `stage_media_inputs` for all selected visual and audio assets.
4. Create `RemotionTimelinePlan` with `schema=clipanvil.remotion_timeline.v1` and `composition=MarketingTimeline`.
5. Call `create_timeline_plan` with `template_key=remotion_timeline_v1`.
6. Call `render_timeline_template` with the same plan.
7. Call `submit_composition_artifact` after receiving a valid `/workspace/output/*.mp4`.

## Quality Bar

- Segment order follows cue_plan order.
- Segment timing follows cue windows scaled to the actual voiceover duration.
- Caption text is user-facing marketing copy, not internal director notes.
- The final plan uses real staged `/workspace` paths for every asset.
- The rendered final video has video, voiceover, BGM, and readable captions.
```

- [ ] **Step 4: Update existing prompts and skills**

In `composer/system_prompt.go`, replace the Phase 1 sentence with:

```text
Supported templates are simple_concat, concat_with_fades, and remotion_timeline_v1. Use remotion_timeline_v1 for low-cost final marketing videos assembled from Seedream stills, voiceover, BGM, captions, and Remotion layout/motion.
```

Add:

```text
For remotion_timeline_v1, still assets are first-class visual inputs. Do not require shot_video or motion_shot_video when cue-matched still images exist. Use AudioPlan cue_plan as the primary timing contract; each segment should match cue shot_ref, cue timing, cue caption, and the staged asset for that shot. Do not use narrative_purpose, visual_intent, action_text, or camera_intent as captions.
```

In Producer prompt, replace the old no-Seedance mandatory motion shot wording with:

```text
For no-Seedance low-cost final video requests, the preferred route is Seedream still images plus Volcengine voiceover/BGM plus Composer remotion_timeline_v1. Do not require every shot to become motion_shot_video. Use motion_shot_video only when a per-shot video artifact is explicitly useful; otherwise let the final Remotion timeline animate stills, captions, transitions, and audio.

When the stills, voiceover, and BGM are ready, dispatch_composer with template_key=remotion_timeline_v1. The instructions must say to use AudioPlan cue_plan for timing, captions, and visual alignment.
```

Update `commerce-ad-producer` and `motion-shot-producer` with the same route language, keeping existing dynamic storyboard rules.

- [ ] **Step 5: Run prompt and skill tests**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills ./internal/agent/producer -run 'RemotionTimelineComposer|MotionShot|CommerceAdProducer|ProducerPrompt|DefaultRegistryContainsCommerceSkillPack' -count=1
```

Expected: PASS.

## Task 4: E2E DB Acceptance Script

**Files:**
- Add: `scripts/smoke-m13-2-agent-remotion-route.sql`

- [ ] **Step 1: Add DB verification SQL**

Create `scripts/smoke-m13-2-agent-remotion-route.sql`:

```sql
\set ON_ERROR_STOP on

-- Usage:
--   psql "$DATABASE_URL" -v workspace_id='<workspace uuid>' -f scripts/smoke-m13-2-agent-remotion-route.sql

select
  'timeline_plan' as check_name,
  count(*) as count
from timeline_plan
where workspace_id = :'workspace_id'::uuid
  and template_key = 'remotion_timeline_v1'
  and status in ('rendered', 'completed', 'succeeded');

select
  'seedance_video_jobs' as check_name,
  count(*) as count
from generation_job
where workspace_id = :'workspace_id'::uuid
  and (
    lower(coalesce(model_id, '')) like '%seedance%'
    or lower(coalesce(model_prompt_profile, '')) like '%seedance%'
  );

select
  'seedream_image_jobs' as check_name,
  count(*) as count
from generation_job
where workspace_id = :'workspace_id'::uuid
  and lower(coalesce(model_prompt_profile, '')) = 'seedream_5_image'
  and status = 'succeeded';

select
  'audio_jobs' as check_name,
  count(*) as count
from generation_job
where workspace_id = :'workspace_id'::uuid
  and lower(coalesce(model_prompt_profile, '')) = 'seed_audio_1'
  and status = 'succeeded';

select
  tp.id,
  tp.template_key,
  tp.status,
  tp.plan_json #>> '{schema}' as schema,
  jsonb_array_length(coalesce(tp.plan_json::jsonb -> 'segments', '[]'::jsonb)) as segment_count,
  tp.artifact_version_id
from timeline_plan tp
where tp.workspace_id = :'workspace_id'::uuid
  and tp.template_key = 'remotion_timeline_v1'
order by tp.created_at desc
limit 3;
```

- [ ] **Step 2: Syntax check SQL file**

```bash
git diff --check -- scripts/smoke-m13-2-agent-remotion-route.sql
```

Expected: PASS.

## Task 5: Verification And Browser E2E

**Files:**
- Modify: `docs/milestones/m13-remotion-timeline-composer.md`

- [ ] **Step 1: Run focused Go tests**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer ./internal/agent/skills ./internal/agent/producer ./internal/agent/tools ./internal/remotiontimeline ./internal/sandbox
```

Expected: PASS.

- [ ] **Step 2: Run build and renderer smoke**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
bash -n scripts/smoke-m13-1-remotion-timeline.sh
node --check sandbox-image/remotion-timeline/src/render.mjs
./scripts/smoke-m13-1-remotion-timeline.sh
git diff --check
```

Expected: PASS. M13.1 renderer smoke should still produce 1080x1920 MP4 with audio stream.

- [ ] **Step 3: Run browser Agent E2E**

Start or reuse the dev server:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
./scripts/dev-start.sh
```

Open the printed Vite URL in the in-app browser, create or use a clean Agent workspace, upload the desktop `box.png`, and send a user prompt equivalent to:

```text
用这张图片作为悦行行李箱商品参考，生成 30 秒以上中文口播营销视频。不要调用 Seedance，不要生成模型视频；可以使用 Seedream 生成多张商品/细节图，可以使用火山引擎生成旁白和 BGM。最终用 Remotion timeline 合成，有字幕、转场、图文动效，并保证讲万向轮时使用轮组/细节图，讲收纳时使用打开箱体/内景图。
```

Wait for final video completion, then verify DB:

```bash
psql "$DATABASE_URL" -v workspace_id='<workspace uuid>' -f scripts/smoke-m13-2-agent-remotion-route.sql
```

Expected:
- `timeline_plan` count is at least 1.
- `seedance_video_jobs` count is 0.
- `seedream_image_jobs` count is at least 2.
- `audio_jobs` count is at least 2.
- latest plan has `schema=clipanvil.remotion_timeline.v1`.

Download the final artifact using the existing signed download API and run:

```bash
ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 '<final.mp4>'
ffprobe -v error -select_streams a:0 -show_entries stream=codec_type -of csv=p=0 '<final.mp4>'
```

Expected:
- duration is `>= 30`.
- audio stream exists.

- [ ] **Step 4: Record M13.2 verification**

In `docs/milestones/m13-remotion-timeline-composer.md`, add a short M13.2 verification record with exact command outputs and E2E workspace id.

Expected: milestone doc states M13.2 completed only after real E2E succeeds.

