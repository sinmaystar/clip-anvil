package reviewer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agentpss "github.com/sinmaystar/clip-anvil/internal/agent/pss"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const maxReviewImageBytes = 10 << 20

type ContextStore interface {
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetGenerationJobByID(ctx context.Context, id pgtype.UUID) (db.GenerationJob, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
	GetRenderPlanByID(ctx context.Context, params db.GetRenderPlanByIDParams) (db.RenderPlan, error)
	ListReviewRecordsByShotPhase(ctx context.Context, params db.ListReviewRecordsByShotPhaseParams) ([]db.ReviewRecord, error)
	ListReviewRecordsByRenderPlan(ctx context.Context, renderPlanID pgtype.UUID) ([]db.ReviewRecord, error)
}

type ImageObjectReader interface {
	ReadObject(ctx context.Context, workspaceID pgtype.UUID, key string, maxBytes int64) ([]byte, storage.ObjectRef, error)
}

type PSSBuilder interface {
	BuildProducerPSS(ctx context.Context, workspaceID pgtype.UUID) (agentpss.ProducerPSS, error)
}

type MessageRuntime interface {
	ListMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error)
}

type ContextLoader struct {
	Store       ContextStore
	Runtime     MessageRuntime
	ImageReader ImageObjectReader
	PSSBuilder  PSSBuilder
}

func (l ContextLoader) Load(ctx context.Context, input GraphInput) (Context, error) {
	if l.Store == nil || !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid {
		return Context{}, ErrInvalidInput
	}
	if input.Task.TargetPhase != TargetPhasePreviewImage &&
		input.Task.TargetPhase != TargetPhaseShotVideo &&
		input.Task.TargetPhase != TargetPhaseFinalVideo &&
		input.Task.TargetPhase != TargetPhasePreRenderPlan {
		return Context{}, fmt.Errorf("%w: unsupported review phase %q", ErrInvalidInput, input.Task.TargetPhase)
	}
	if input.Task.TargetPhase == TargetPhasePreRenderPlan {
		return l.loadPreRenderPlanContext(ctx, input)
	}
	requiresShot := input.Task.TargetPhase != TargetPhaseFinalVideo
	shotID, hasShotID := pgUUIDFromString(input.Task.ShotID)
	if requiresShot && !hasShotID {
		return Context{}, fmt.Errorf("%w: shot_id is required", ErrInvalidInput)
	}
	nodeID, ok := pgUUIDFromString(input.Task.NodeID)
	if !ok {
		return Context{}, fmt.Errorf("%w: node_id is required", ErrInvalidInput)
	}
	versionID, ok := pgUUIDFromString(input.Task.ArtifactVersionID)
	if !ok {
		return Context{}, fmt.Errorf("%w: artifact_version_id is required", ErrInvalidInput)
	}
	shot := db.Shot{}
	if requiresShot {
		var err error
		shot, err = l.Store.GetShotByID(ctx, shotID)
		if err != nil {
			return Context{}, err
		}
	}
	node, err := l.Store.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	version, err := l.Store.GetArtifactVersionByID(ctx, versionID)
	if err != nil {
		return Context{}, err
	}
	if requiresShot && shot.WorkspaceID != input.WorkspaceID {
		return Context{}, ErrInvalidInput
	}
	if node.WorkspaceID != input.WorkspaceID || version.WorkspaceID != input.WorkspaceID {
		return Context{}, ErrInvalidInput
	}
	if requiresShot && node.ShotID.Valid && node.ShotID != shot.ID {
		return Context{}, ErrInvalidInput
	}
	if version.NodeID != node.ID {
		return Context{}, ErrInvalidInput
	}
	job := db.GenerationJob{}
	if jobID, ok := pgUUIDFromString(input.Task.GenerationJobID); ok {
		job, err = l.Store.GetGenerationJobByID(ctx, jobID)
		if err != nil {
			return Context{}, err
		}
		if job.WorkspaceID.Valid && job.WorkspaceID != input.WorkspaceID {
			return Context{}, ErrInvalidInput
		}
		if job.TargetNodeID.Valid && job.TargetNodeID != node.ID {
			return Context{}, ErrInvalidInput
		}
	}
	assetURL, assetMime := "", ""
	if version.AssetID.Valid {
		asset, err := l.Store.GetMediaAssetByID(ctx, version.AssetID)
		if err != nil {
			return Context{}, err
		}
		assetURL, assetMime = l.modelAssetReference(ctx, asset)
	}
	var priorReviews []db.ReviewRecord
	if requiresShot {
		var err error
		priorReviews, err = l.Store.ListReviewRecordsByShotPhase(ctx, db.ListReviewRecordsByShotPhaseParams{
			WorkspaceID: input.WorkspaceID,
			ShotID:      shot.ID,
			TargetPhase: input.Task.TargetPhase,
		})
		if err != nil {
			return Context{}, err
		}
	}
	productionText := ""
	if l.PSSBuilder != nil {
		pss, err := l.PSSBuilder.BuildProducerPSS(ctx, input.WorkspaceID)
		if err != nil {
			return Context{}, err
		}
		productionText = pss.Text
	}
	out := Context{
		Input:          input,
		Shot:           shot,
		Node:           node,
		Version:        version,
		GenerationJob:  job,
		Messages:       nil,
		PriorReviews:   priorReviews,
		ProductionText: productionText,
		AssetURL:       assetURL,
		AssetMime:      assetMime,
	}
	out.Text = buildReviewContextText(out)
	return out, nil
}

func (l ContextLoader) loadPreRenderPlanContext(ctx context.Context, input GraphInput) (Context, error) {
	renderPlanID, ok := pgUUIDFromString(input.Task.Target.RenderPlanID)
	if !ok {
		return Context{}, fmt.Errorf("%w: render_plan_id is required", ErrInvalidInput)
	}
	plan, err := l.Store.GetRenderPlanByID(ctx, db.GetRenderPlanByIDParams{
		WorkspaceID: input.WorkspaceID,
		ID:          renderPlanID,
	})
	if err != nil {
		return Context{}, err
	}
	if plan.WorkspaceID != input.WorkspaceID {
		return Context{}, ErrInvalidInput
	}
	shot := db.Shot{}
	if plan.ScopeType == "shot" && plan.ScopeID.Valid {
		if loadedShot, err := l.Store.GetShotByID(ctx, plan.ScopeID); err == nil && loadedShot.WorkspaceID == input.WorkspaceID {
			shot = loadedShot
		}
	}
	priorReviews, err := l.Store.ListReviewRecordsByRenderPlan(ctx, plan.ID)
	if err != nil {
		return Context{}, err
	}
	productionText := ""
	if l.PSSBuilder != nil {
		pss, err := l.PSSBuilder.BuildProducerPSS(ctx, input.WorkspaceID)
		if err != nil {
			return Context{}, err
		}
		productionText = pss.Text
	}
	out := Context{
		Input:          input,
		Shot:           shot,
		RenderPlan:     plan,
		PriorReviews:   priorReviews,
		ProductionText: productionText,
	}
	out.Text = buildReviewContextText(out)
	return out, nil
}

func (l ContextLoader) modelAssetReference(ctx context.Context, asset db.MediaAsset) (string, string) {
	rawURL := strings.TrimSpace(asset.StorageUrl.String)
	mime := strings.TrimSpace(asset.Mime)
	if rawURL == "" {
		return "", mime
	}
	if isModelAssetReference(rawURL) {
		return rawURL, mime
	}
	if mime != "" && !strings.HasPrefix(mime, "image/") {
		return rawURL, mime
	}
	if l.ImageReader == nil {
		return "", mime
	}
	key, err := storage.KeyFromStorageURL(asset.WorkspaceID, rawURL)
	if err != nil {
		key = strings.TrimPrefix(rawURL, "minio://")
	}
	data, ref, err := l.ImageReader.ReadObject(ctx, asset.WorkspaceID, key, maxReviewImageBytes)
	if err != nil || len(data) == 0 {
		return "", mime
	}
	if mime == "" {
		mime = strings.TrimSpace(ref.MIME)
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), mime
}

func buildReviewContextText(reviewContext Context) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review Target\n")
	fmt.Fprintf(&b, "- phase: %s\n", reviewContext.Input.Task.TargetPhase)
	if reviewContext.RenderPlan.ID.Valid {
		fmt.Fprintf(&b, "- render_plan_ref: render_plan/%s target_phase=%s operation=%s profile=%s status=%s\n", semanticOrFallback(reviewContext.RenderPlan.SemanticKey, reviewContext.RenderPlan.RenderPlanKey), reviewContext.RenderPlan.TargetPhase, reviewContext.RenderPlan.Operation, reviewContext.RenderPlan.ModelPromptProfile, reviewContext.RenderPlan.Status)
		fmt.Fprintf(&b, "- render_plan_scope: %s/%s revision=%d\n", reviewContext.RenderPlan.ScopeType, uuidString(reviewContext.RenderPlan.ScopeID), reviewContext.RenderPlan.Revision)
		writeJSONSummary(&b, "params", reviewContext.RenderPlan.Params)
		writeJSONSummary(&b, "audit_hints", reviewContext.RenderPlan.AuditHints)
		writeJSONSummary(&b, "compiled_request", reviewContext.RenderPlan.CompiledRequest)
		writeJSONSummary(&b, "cost_estimate", reviewContext.RenderPlan.CostEstimate)
		if strings.TrimSpace(reviewContext.RenderPlan.Rationale) != "" {
			fmt.Fprintf(&b, "- rationale: %s\n", strings.TrimSpace(reviewContext.RenderPlan.Rationale))
		}
	}
	if reviewContext.Shot.ID.Valid {
		fmt.Fprintf(&b, "- shot_ref: shot/%s %s status=%s\n", semanticOrFallback(reviewContext.Shot.SemanticKey, reviewContext.Shot.ClientKey), reviewContext.Shot.Title, reviewContext.Shot.Status)
	}
	if reviewContext.Node.ID.Valid {
		fmt.Fprintf(&b, "- node_ref: media_node/%s type=%s status=%s\n", semanticOrFallback(reviewContext.Node.SemanticKey, reviewContext.Node.Title), reviewContext.Node.NodeType, reviewContext.Node.Status)
	}
	if reviewContext.Version.ID.Valid {
		fmt.Fprintf(&b, "- artifact_version_ref: artifact_version/%s v%d status=%s\n", semanticOrFallback(reviewContext.Version.SemanticKey, reviewContext.Version.DisplayName), reviewContext.Version.VersionNo, reviewContext.Version.Status)
	}
	if strings.TrimSpace(reviewContext.GenerationJob.RenderedPrompt) != "" {
		fmt.Fprintf(&b, "- prompt: %s\n", strings.TrimSpace(reviewContext.GenerationJob.RenderedPrompt))
	}
	if routeFacts := routeFactsText(reviewContext); routeFacts != "" {
		fmt.Fprintf(&b, "\nRoute Facts\n%s\n", routeFacts)
	}
	if reviewContext.Input.Task.TargetPhase == TargetPhaseFinalVideo {
		fmt.Fprintf(&b, "\nFinal Audio Review Focus\n")
		fmt.Fprintf(&b, "- required_focus: audio_sync, platform_selling_power, voiceover/BGM presence, relative volume, BGM ducking, timing continuity, marketing objective support\n")
		if audio := audioJSONSummary(reviewContext.Node.Metadata); audio != "" {
			fmt.Fprintf(&b, "- node_audio: %s\n", audio)
		}
		if audio := audioJSONSummary(reviewContext.Version.Output); audio != "" {
			fmt.Fprintf(&b, "- artifact_audio: %s\n", audio)
		}
	}
	if len(reviewContext.PriorReviews) > 0 {
		fmt.Fprintf(&b, "\nPrior Reviews\n")
		for _, record := range reviewContext.PriorReviews {
			fmt.Fprintf(&b, "- %s score=%s critique=%s\n", record.Status, floatText(record.OverallScore), record.Critique)
		}
	}
	if strings.TrimSpace(reviewContext.ProductionText) != "" {
		fmt.Fprintf(&b, "\nProduction State\n%s\n", strings.TrimSpace(reviewContext.ProductionText))
	}
	return b.String()
}

func writeJSONSummary(b *strings.Builder, label string, raw []byte) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" || trimmed == "[]" {
		return
	}
	var value any
	if err := json.Unmarshal(raw, &value); err == nil {
		if compact, err := json.Marshal(value); err == nil {
			trimmed = string(compact)
		}
	}
	if len(trimmed) > 1200 {
		trimmed = trimmed[:1200] + "...(truncated)"
	}
	fmt.Fprintf(b, "- %s: %s\n", label, trimmed)
}

func routeFactsText(reviewContext Context) string {
	facts := routeFacts{
		Provider:      firstNonEmpty(reviewContext.GenerationJob.Provider, reviewContext.Node.ModelProvider.String),
		ModelID:       firstNonEmpty(reviewContext.GenerationJob.ModelID, reviewContext.Node.ModelID.String),
		OperationType: strings.TrimSpace(reviewContext.GenerationJob.OperationType),
	}
	for _, raw := range [][]byte{
		reviewContext.Node.Metadata,
		reviewContext.Version.Output,
		reviewContext.Version.ProviderResponse,
		reviewContext.GenerationJob.ProviderResponse,
	} {
		mergeRouteFacts(&facts, raw)
	}
	if facts.empty() {
		return ""
	}
	lines := []string{}
	if facts.Provider != "" || facts.ModelID != "" || facts.OperationType != "" {
		lines = append(lines, fmt.Sprintf("- provider=%s model_id=%s operation_type=%s", facts.Provider, facts.ModelID, facts.OperationType))
	}
	if facts.RenderingFamily != "" || facts.TemplateEngine != "" || facts.TemplateKey != "" {
		lines = append(lines, fmt.Sprintf("- rendering_family=%s template_engine=%s template_key=%s", facts.RenderingFamily, facts.TemplateEngine, facts.TemplateKey))
	}
	if facts.RenderingFamily == "template_video" || facts.TemplateEngine != "" || facts.TemplateKey != "" || facts.Provider == "internal_template_video" {
		lines = append(lines, "- review_focus=readability, platform_selling_power, brand_consistency, motion_rhythm, audio_sync, truthfulness")
	}
	return strings.Join(lines, "\n")
}

type routeFacts struct {
	Provider        string
	ModelID         string
	OperationType   string
	RenderingFamily string
	TemplateEngine  string
	TemplateKey     string
}

func (f routeFacts) empty() bool {
	return f.Provider == "" &&
		f.ModelID == "" &&
		f.OperationType == "" &&
		f.RenderingFamily == "" &&
		f.TemplateEngine == "" &&
		f.TemplateKey == ""
}

func mergeRouteFacts(facts *routeFacts, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	facts.Provider = firstNonEmpty(facts.Provider, jsonString(payload, "provider"))
	facts.ModelID = firstNonEmpty(facts.ModelID, jsonString(payload, "model_id"))
	facts.OperationType = firstNonEmpty(facts.OperationType, jsonString(payload, "operation_type"))
	facts.RenderingFamily = firstNonEmpty(facts.RenderingFamily, jsonString(payload, "rendering_family"))
	facts.TemplateEngine = firstNonEmpty(facts.TemplateEngine, jsonString(payload, "template_engine"))
	facts.TemplateKey = firstNonEmpty(facts.TemplateKey, jsonString(payload, "template_key"))
}

func jsonString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func audioJSONSummary(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	summary := map[string]any{}
	for _, key := range []string{"audio_tracks", "audio_codec", "duration_sec", "mix"} {
		if value, ok := payload[key]; ok {
			summary[key] = value
		}
	}
	if len(summary) == 0 {
		return ""
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return ""
	}
	if len(encoded) > 1200 {
		return string(encoded[:1200]) + "...truncated"
	}
	return string(encoded)
}

func semanticOrFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback
	}
	return "semantic_key_missing"
}

func isModelAssetReference(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "data:image/")
}

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func floatText(value pgtype.Float4) string {
	if !value.Valid {
		return "-"
	}
	return fmt.Sprintf("%.2f", value.Float32)
}
