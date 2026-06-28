package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestGenerationIntentJSONShape(t *testing.T) {
	workspaceID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	nodeID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	intent := GenerationIntent{
		WorkspaceID:    workspaceID,
		TargetNodeID:   nodeID,
		OutputType:     "text",
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		InputRefs: []InputRef{
			{NodeID: nodeID, Kind: "dependency", Required: false},
		},
		Model: ModelSpec{
			Provider: "mock",
			ModelID:  "mock-text",
		},
		Params: map[string]any{"temperature": 0.2},
		RequestedBy: RequestedBy{
			Type: "user",
			ID:   "account-123",
		},
	}

	raw, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	if got["operation_type"] != "text_generation" {
		t.Fatalf("operation_type = %v", got["operation_type"])
	}
	model := got["model"].(map[string]any)
	if model["provider"] != "mock" || model["model_id"] != "mock-text" {
		t.Fatalf("model = %#v", model)
	}
	requestedBy := got["requested_by"].(map[string]any)
	if requestedBy["type"] != "user" || requestedBy["id"] != "account-123" {
		t.Fatalf("requested_by = %#v", requestedBy)
	}
	if _, ok := got["model_provider"]; ok {
		t.Fatalf("intent must use nested model, got legacy model_provider")
	}
}

func TestAgentSemanticKeysForRenderPlanIntent(t *testing.T) {
	intent := GenerationIntent{
		OutputType: "image",
		Semantic: SemanticInfo{
			RenderPlanKey: "scene_main.shot_01.preview_image.r1",
			ArtifactKind:  "preview_image",
		},
	}

	if got := generationJobSemanticKey(intent, 2); got != "scene_main.shot_01.preview_image.r1.job.a2" {
		t.Fatalf("job semantic key = %q", got)
	}
	if got := artifactVersionSemanticKey(intent, 3); got != "scene_main.shot_01.preview_image.r1.artifact.v3" {
		t.Fatalf("artifact semantic key = %q", got)
	}
	if got := artifactKindForIntent(intent); got != "preview_image" {
		t.Fatalf("artifact kind = %q", got)
	}
}

func TestComposerArtifactDefaultsToFinalVideoSemanticIdentity(t *testing.T) {
	node := db.MediaNode{
		ID:          pgtype.UUID{Bytes: [16]byte{0xab}, Valid: true},
		SemanticKey: "final_video.c69041ee.node",
	}
	if got := composerArtifactRenderKey(node); got != "final_video.c69041ee.compose" {
		t.Fatalf("composer render key = %q", got)
	}
	intent := GenerationIntent{
		OutputType: "video",
		Semantic: SemanticInfo{
			RenderPlanKey: composerArtifactRenderKey(node),
			ArtifactKind:  "final_video",
		},
	}
	if got := artifactKindForIntent(intent); got != "final_video" {
		t.Fatalf("artifact kind = %q", got)
	}
	if got := generationJobSemanticKey(intent, 1); got != "final_video.c69041ee.compose.job.a1" {
		t.Fatalf("job semantic key = %q", got)
	}
	if got := artifactVersionSemanticKey(intent, 1); got != "final_video.c69041ee.compose.artifact.v1" {
		t.Fatalf("artifact semantic key = %q", got)
	}
	defaulted := withComposerSemanticDefaults(node, GenerationIntent{
		OutputType:    "video",
		OperationType: "compose_final_video",
	})
	if defaulted.Semantic.ArtifactKind != "final_video" || defaulted.Semantic.RenderPlanKey != "final_video.c69041ee.compose" {
		t.Fatalf("composer semantic defaults = %#v", defaulted.Semantic)
	}
}

func TestComposerArtifactHelperUsesProductionPersistencePath(t *testing.T) {
	var _ interface {
		PersistComposerArtifact(context.Context, ComposerArtifactInput) (RunResult, error)
	} = (*Service)(nil)

	raw, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{
		"type ComposerArtifactInput struct",
		"func (s *Service) PersistComposerArtifact(ctx context.Context, input ComposerArtifactInput) (RunResult, error)",
		"createQueuedJobWithVersion",
		"persistQueuedJobSuccess",
		"input.SandboxJobID",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("service.go missing %q", want)
		}
	}
}

func TestProviderRegistrySelectsMockProvider(t *testing.T) {
	registry := NewProviderRegistry(ProviderConfig{
		ProviderMode:     "mock",
		DefaultProvider:  "mock",
		DefaultTextModel: "mock-text",
	})

	intent := GenerationIntent{
		PromptTemplate: "write a short ad",
		OutputType:     "text",
		OperationType:  "text_generation",
		Model:          ModelSpec{},
	}

	resolved := registry.ApplyDefaults(intent)
	if resolved.Model.Provider != "mock" {
		t.Fatalf("provider = %q, want mock", resolved.Model.Provider)
	}
	if resolved.Model.ModelID != "mock-text" {
		t.Fatalf("model = %q, want mock-text", resolved.Model.ModelID)
	}

	provider, err := registry.Resolve(resolved)
	if err != nil {
		t.Fatalf("resolve provider: %v", err)
	}
	result, err := provider.Run(context.Background(), resolved)
	if err != nil {
		t.Fatalf("run mock provider: %v", err)
	}
	if result.RenderedPrompt != "write a short ad" {
		t.Fatalf("rendered prompt = %q", result.RenderedPrompt)
	}
}

func TestMockProviderUsesRenderedPrompt(t *testing.T) {
	result, err := (MockProvider{}).Run(context.Background(), GenerationIntent{
		PromptTemplate: "use @视频脚本",
		RenderedPrompt: "[视频脚本]\n第一幕：机场大厅。",
		OutputType:     "text",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.TextContent, "@视频脚本") {
		t.Fatalf("text content = %q, should not contain raw prompt ref mention", result.TextContent)
	}
	if !strings.Contains(result.TextContent, "第一幕：机场大厅。") {
		t.Fatalf("text content = %q, want rendered prompt content", result.TextContent)
	}
}

func TestProviderRegistryRejectsUnknownProvider(t *testing.T) {
	registry := NewProviderRegistry(ProviderConfig{
		ProviderMode:     "mock",
		DefaultProvider:  "mock",
		DefaultTextModel: "mock-text",
	})

	_, err := registry.Resolve(GenerationIntent{
		Model: ModelSpec{Provider: "unknown", ModelID: "model"},
	})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("error = %v, want ErrProviderUnavailable", err)
	}
}

func TestVolcengineProviderFailsBeforeNetworkWithoutAPIKey(t *testing.T) {
	provider := NewVolcengineProvider(VolcengineProviderConfig{
		BaseURL:   "https://example.invalid",
		TextModel: "doubao-cheap",
	})

	_, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:     "text",
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "volcengine", ModelID: "doubao-cheap"},
		Params:         map[string]any{},
	})
	if !errors.Is(err, ErrProviderConfig) {
		t.Fatalf("error = %v, want ErrProviderConfig", err)
	}
}

func TestMockProviderReturnsDeterministicText(t *testing.T) {
	provider := MockProvider{}
	intent := GenerationIntent{
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:         map[string]any{},
	}

	result, err := provider.Run(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}

	if result.RenderedPrompt != "write a short ad" {
		t.Fatalf("rendered prompt = %q, want write a short ad", result.RenderedPrompt)
	}
	if result.TextContent != "[mock:mock-text] write a short ad" {
		t.Fatalf("text content = %q, want mock text", result.TextContent)
	}
}

func TestErrorCodeForProviderConfig(t *testing.T) {
	err := fmt.Errorf("%w: missing key", ErrProviderConfig)
	if code := errorCodeForRun(err); code != "provider_config_error" {
		t.Fatalf("code = %q, want provider_config_error", code)
	}
}

func TestIntentForNodeUsesProductionFields(t *testing.T) {
	node := db.MediaNode{
		NodeType:       db.NodeTypeText,
		OperationType:  "text_generation",
		PromptTemplate: "write a crisp line",
		ModelProvider:  pgtype.Text{String: "mock", Valid: true},
		ModelID:        pgtype.Text{String: "mock-text", Valid: true},
		ModelParams:    []byte(`{"temperature":0.2}`),
	}

	intent := intentForNode(node, RequestedBy{Type: "user", ID: "account-123"})
	if intent.Model.Provider != "mock" {
		t.Fatalf("provider = %q", intent.Model.Provider)
	}
	if intent.Model.ModelID != "mock-text" {
		t.Fatalf("model = %q", intent.Model.ModelID)
	}
	if intent.Params["temperature"] != 0.2 {
		t.Fatalf("params = %#v", intent.Params)
	}
	if intent.RequestedBy.ID != "account-123" {
		t.Fatalf("requested by = %#v", intent.RequestedBy)
	}
}

func TestCapabilityValidatorAcceptsSupportedIntent(t *testing.T) {
	capability := Capability{
		ProviderID:              "mock",
		ModelID:                 "mock-text",
		OutputTypes:             []string{"text"},
		SupportedOperations:     []string{"text_generation"},
		SupportedInputNodeTypes: []string{"text"},
		Limits:                  CapabilityLimits{MaxPromptChars: 100, MaxAttempts: 3},
	}
	intent := GenerationIntent{
		OutputType:     "text",
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:         map[string]any{},
	}

	if err := ValidateCapability(intent, capability); err != nil {
		t.Fatalf("ValidateCapability() error = %v", err)
	}
}

func TestCapabilityValidatorRejectsOutputMismatch(t *testing.T) {
	capability := Capability{
		ProviderID:          "mock",
		ModelID:             "mock-image-only",
		OutputTypes:         []string{"image"},
		SupportedOperations: []string{"text_to_image"},
		Limits:              CapabilityLimits{MaxAttempts: 3},
	}
	intent := GenerationIntent{
		OutputType:     "video",
		OperationType:  "text_to_video",
		PromptTemplate: "make a video",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-image-only"},
		Params:         map[string]any{},
	}

	err := ValidateCapability(intent, capability)
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("error = %v, want ErrCapabilityMismatch", err)
	}
	if code := errorCodeForRun(err); code != "capability_mismatch" {
		t.Fatalf("code = %q, want capability_mismatch", code)
	}
}

func TestCapabilityValidatorRejectsLimitMismatch(t *testing.T) {
	capability := Capability{
		ProviderID:          "mock",
		ModelID:             "mock-video",
		OutputTypes:         []string{"video"},
		SupportedOperations: []string{"text_to_video"},
		Limits: CapabilityLimits{
			MaxPromptChars:   100,
			MaxAttempts:      3,
			AllowedDurations: []int{4, 5, 8},
		},
	}
	intent := GenerationIntent{
		OutputType:     "video",
		OperationType:  "text_to_video",
		PromptTemplate: "make a video",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-video"},
		Params:         map[string]any{"duration_sec": float64(15)},
	}

	err := ValidateCapability(intent, capability)
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("error = %v, want ErrCapabilityMismatch", err)
	}
}

func TestCapabilityValidatorRejectsUnsupportedInputNodeType(t *testing.T) {
	capability := Capability{
		ProviderID:              "mock",
		ModelID:                 "mock-image-only",
		OutputTypes:             []string{"image"},
		SupportedOperations:     []string{"text_to_image"},
		SupportedInputNodeTypes: []string{"text"},
		Limits:                  CapabilityLimits{MaxAttempts: 3},
	}
	intent := GenerationIntent{
		OutputType:     "image",
		OperationType:  "text_to_image",
		PromptTemplate: "make an image",
		InputRefs: []InputRef{
			{Kind: "dependency", NodeType: "video"},
		},
		Model:  ModelSpec{Provider: "mock", ModelID: "mock-image-only"},
		Params: map[string]any{},
	}

	err := ValidateCapability(intent, capability)
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("error = %v, want ErrCapabilityMismatch", err)
	}
	if !strings.Contains(err.Error(), "does not support input node type video") {
		t.Fatalf("error = %q, want unsupported input node type message", err.Error())
	}
}

func TestMarkSubmittedRenderPlanTerminalUsesAgentWorkerTaskID(t *testing.T) {
	syncer := &fakeRenderPlanTerminalSyncer{}
	job := db.GenerationJob{
		WorkspaceID:     pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true},
		RequestedByType: "agent_worker",
		RequestedByID:   pgtype.Text{String: "02000000-0000-0000-0000-000000000000", Valid: true},
	}
	versionID := pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true}
	nodeID := pgtype.UUID{Bytes: [16]byte{0x04}, Valid: true}

	if err := markSubmittedRenderPlanTerminal(context.Background(), syncer, job, "failed", versionID, nodeID); err != nil {
		t.Fatal(err)
	}
	if len(syncer.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(syncer.calls))
	}
	call := syncer.calls[0]
	if call.SubmittedWorkerTaskID != (pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true}) || call.Status != "failed" || call.OutputVersionID != versionID || call.OutputNodeID != nodeID {
		t.Fatalf("call = %#v", call)
	}
}

func TestMarkSubmittedRenderPlanTerminalIgnoresNonAgentWorkerJobs(t *testing.T) {
	syncer := &fakeRenderPlanTerminalSyncer{}
	job := db.GenerationJob{
		WorkspaceID:     pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true},
		RequestedByType: "user",
		RequestedByID:   pgtype.Text{String: "02000000-0000-0000-0000-000000000000", Valid: true},
	}

	if err := markSubmittedRenderPlanTerminal(context.Background(), syncer, job, "succeeded", pgtype.UUID{}, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	if len(syncer.calls) != 0 {
		t.Fatalf("calls = %#v", syncer.calls)
	}
}

func TestPromptRefKindForDirectDependency(t *testing.T) {
	depID := pgtype.UUID{Bytes: [16]byte{0x31}, Valid: true}
	target := db.MediaNode{
		ID:         pgtype.UUID{Bytes: [16]byte{0x32}, Valid: true},
		PromptRefs: []byte(`{"version":1,"refs":[{"node_id":"31000000-0000-0000-0000-000000000000","label":"A","node_type":"image"}]}`),
	}
	dep := db.MediaNode{ID: depID, NodeType: db.NodeTypeImage}

	if got := inputKindForDependency(target, dep); got != InputKindExplicit {
		t.Fatalf("kind = %q, want explicit", got)
	}
}

type fakeRenderPlanTerminalSyncer struct {
	calls []db.MarkSubmittedRenderPlanCompletedByWorkerTaskParams
}

func (f *fakeRenderPlanTerminalSyncer) MarkSubmittedRenderPlanCompletedByWorkerTask(_ context.Context, params db.MarkSubmittedRenderPlanCompletedByWorkerTaskParams) (db.RenderPlan, error) {
	f.calls = append(f.calls, params)
	return db.RenderPlan{WorkspaceID: params.WorkspaceID, SubmittedWorkerTaskID: params.SubmittedWorkerTaskID, Status: params.Status}, nil
}

func TestPromptRefKindForImplicitDependency(t *testing.T) {
	target := db.MediaNode{
		ID:         pgtype.UUID{Bytes: [16]byte{0x33}, Valid: true},
		PromptRefs: []byte(`{"version":1,"refs":[]}`),
	}
	dep := db.MediaNode{ID: pgtype.UUID{Bytes: [16]byte{0x34}, Valid: true}, NodeType: db.NodeTypeImage}

	if got := inputKindForDependency(target, dep); got != InputKindImplicit {
		t.Fatalf("kind = %q, want implicit", got)
	}
}

func TestPromptRefsInvalidWhenRefIsNotUpstream(t *testing.T) {
	target := db.MediaNode{
		ID:         pgtype.UUID{Bytes: [16]byte{0x35}, Valid: true},
		PromptRefs: []byte(`{"version":1,"refs":[{"node_id":"36000000-0000-0000-0000-000000000000","label":"Missing edge","node_type":"image"}]}`),
	}
	upstream := []db.MediaNode{
		{ID: pgtype.UUID{Bytes: [16]byte{0x37}, Valid: true}, NodeType: db.NodeTypeImage},
	}

	invalid, err := invalidPromptRefs(target, upstream)
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) != 1 {
		t.Fatalf("invalid len = %d, want 1", len(invalid))
	}
	if invalid[0].NodeID != "36000000-0000-0000-0000-000000000000" {
		t.Fatalf("invalid node id = %q", invalid[0].NodeID)
	}
}

func TestRenderPromptRefsExpandsTextDependencyWinner(t *testing.T) {
	depID := pgtype.UUID{Bytes: [16]byte{0x31}, Valid: true}
	intent := GenerationIntent{
		PromptTemplate: "根据如下脚本生成九宫格图片\n\n@视频脚本",
	}
	inputs := []InputRef{
		{
			NodeID:      depID,
			Kind:        InputKindExplicit,
			NodeType:    "text",
			AssetType:   "text",
			TextContent: "第一幕：机场大厅。\n第二幕：三个行李箱。",
		},
	}

	rendered, err := renderPromptRefs(intent, []byte(`{"version":1,"refs":[{"node_id":"31000000-0000-0000-0000-000000000000","label":"视频脚本","node_type":"text"}]}`), inputs)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(rendered.RenderedPrompt, "@视频脚本") {
		t.Fatalf("rendered prompt still contains prompt ref mention: %q", rendered.RenderedPrompt)
	}
	if !strings.Contains(rendered.RenderedPrompt, "第一幕：机场大厅。") {
		t.Fatalf("rendered prompt = %q, want dependency text content", rendered.RenderedPrompt)
	}
	if rendered.PromptTemplate != intent.PromptTemplate {
		t.Fatalf("prompt template = %q, want original template", rendered.PromptTemplate)
	}
}

func TestRenderPromptRefsReplacesImageDependencyWithStableAlias(t *testing.T) {
	imageID := pgtype.UUID{Bytes: [16]byte{0x29}, Valid: true}
	textID := pgtype.UUID{Bytes: [16]byte{0x31}, Valid: true}
	intent := GenerationIntent{
		PromptTemplate: "根据 @视频脚本 和 @分镜图 生成TVC视频",
		InputRefs: []InputRef{
			{
				NodeID:     imageID,
				Kind:       InputKindExplicit,
				NodeType:   "image",
				AssetType:  "image",
				StorageURL: "workspace-a/production/storyboard.jpg",
			},
			{
				NodeID:      textID,
				Kind:        InputKindExplicit,
				NodeType:    "text",
				AssetType:   "text",
				TextContent: "第一幕：机场大厅。",
			},
		},
	}

	rendered, err := renderPromptRefs(intent, []byte(`{"version":1,"refs":[{"node_id":"31000000-0000-0000-0000-000000000000","label":"视频脚本","node_type":"text"},{"node_id":"29000000-0000-0000-0000-000000000000","label":"分镜图","node_type":"image"}]}`), intent.InputRefs)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(rendered.RenderedPrompt, "@视频脚本") || strings.Contains(rendered.RenderedPrompt, "@分镜图") {
		t.Fatalf("rendered prompt still contains prompt ref mention: %q", rendered.RenderedPrompt)
	}
	if !strings.Contains(rendered.RenderedPrompt, "第一幕：机场大厅。") {
		t.Fatalf("rendered prompt = %q, want text dependency content", rendered.RenderedPrompt)
	}
	if !strings.Contains(rendered.RenderedPrompt, "图1") {
		t.Fatalf("rendered prompt = %q, want image alias", rendered.RenderedPrompt)
	}
	if len(rendered.InputRefs) != 2 || rendered.InputRefs[1].NodeType != "image" {
		t.Fatalf("input refs = %#v, want image ref preserved after text ref", rendered.InputRefs)
	}
}

type fakeSourceInputQueries struct {
	asset db.MediaAsset
}

func (q fakeSourceInputQueries) ListUpstreamDependencyNodes(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return nil, nil
}

func (q fakeSourceInputQueries) ListReferencePackItemNodes(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return nil, nil
}

func (q fakeSourceInputQueries) GetArtifactVersionByID(context.Context, pgtype.UUID) (db.ArtifactVersion, error) {
	return db.ArtifactVersion{}, errors.New("unexpected version lookup")
}

func (q fakeSourceInputQueries) GetMediaAssetByID(context.Context, pgtype.UUID) (db.MediaAsset, error) {
	return q.asset, nil
}

func TestLoadNodeInputFactUsesManualTextSourceNode(t *testing.T) {
	node := db.MediaNode{
		ID:            pgtype.UUID{Bytes: [16]byte{0x41}, Valid: true},
		NodeType:      db.NodeTypeText,
		Title:         "视频脚本",
		Prompt:        "第一幕：机场大厅。\n第二幕：商品特写。",
		OperationType: "manual",
		Status:        db.NodeStatusSucceeded,
	}

	fact, ref, err := loadNodeInputFact(context.Background(), fakeSourceInputQueries{}, node, InputKindExplicit)
	if err != nil {
		t.Fatalf("resolve source text ref: %v", err)
	}
	if ref.TextContent != node.Prompt {
		t.Fatalf("text content = %q, want prompt content", ref.TextContent)
	}
	if ref.AssetType != "text" {
		t.Fatalf("asset type = %q, want text", ref.AssetType)
	}
	if fact.InputHash == "" {
		t.Fatal("manual text source should contribute input hash")
	}
}

func TestLoadNodeInputFactUsesUploadedAssetNode(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x42}, Valid: true}
	asset := db.MediaAsset{
		ID:         assetID,
		Type:       db.AssetTypeImage,
		Mime:       "image/png",
		StorageUrl: pgtype.Text{String: "workspace-a/assets/product.png", Valid: true},
	}
	node := db.MediaNode{
		ID:            pgtype.UUID{Bytes: [16]byte{0x43}, Valid: true},
		NodeType:      db.NodeTypeImage,
		Title:         "商品主图",
		OperationType: "upload",
		AssetID:       assetID,
		Status:        db.NodeStatusSucceeded,
	}

	fact, ref, err := loadNodeInputFact(context.Background(), fakeSourceInputQueries{asset: asset}, node, InputKindExplicit)
	if err != nil {
		t.Fatalf("resolve upload source ref: %v", err)
	}
	if ref.AssetID != uuidToString(assetID) || ref.StorageURL != asset.StorageUrl.String {
		t.Fatalf("asset ref = %#v, want uploaded asset", ref)
	}
	if ref.AssetType != "image" || ref.Mime != "image/png" {
		t.Fatalf("asset type/mime = %q/%q, want image/png", ref.AssetType, ref.Mime)
	}
	if fact.InputHash == "" {
		t.Fatal("uploaded source asset should contribute input hash")
	}
}

func TestMaxAttemptsRespectsCapabilityLimit(t *testing.T) {
	options := RunOptions{MaxAttempts: 10}
	capability := Capability{Limits: CapabilityLimits{MaxAttempts: 3}}
	if got := maxAttemptsForRun(options, capability); got != 3 {
		t.Fatalf("max attempts = %d, want 3", got)
	}
}

func TestChangedInputStaleReasonDetails(t *testing.T) {
	upstreamNodeID := pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true}
	upstreamVersionID := pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true}
	targetVersionID := pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true}

	details, err := changedInputStaleReasonDetails(upstreamNodeID, upstreamVersionID, targetVersionID, "sha256:old", "sha256:new")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(details, &got); err != nil {
		t.Fatal(err)
	}
	if got["upstream_node_id"] != uuidToString(upstreamNodeID) {
		t.Fatalf("upstream_node_id = %v", got["upstream_node_id"])
	}
	if got["target_version_id"] != uuidToString(targetVersionID) {
		t.Fatalf("target_version_id = %v", got["target_version_id"])
	}
	if got["previous_input_hash"] != "sha256:old" || got["current_input_hash"] != "sha256:new" {
		t.Fatalf("hash details = %#v", got)
	}
	if got["reason"] != "upstream_current_version_changed" {
		t.Fatalf("reason = %v", got["reason"])
	}
}

func TestChangedInputStaleReasonDetailsSupportsReferencePackReason(t *testing.T) {
	sourceNodeID := pgtype.UUID{Bytes: [16]byte{0x07}, Valid: true}
	sourceVersionID := pgtype.UUID{Bytes: [16]byte{0x08}, Valid: true}
	targetVersionID := pgtype.UUID{Bytes: [16]byte{0x09}, Valid: true}

	details, err := changedInputStaleReasonDetailsWithReason(
		sourceNodeID,
		sourceVersionID,
		targetVersionID,
		"sha256:old",
		"sha256:new",
		"reference_pack_member_version_changed",
	)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(details, &got); err != nil {
		t.Fatal(err)
	}
	if got["reason"] != "reference_pack_member_version_changed" {
		t.Fatalf("reason = %v", got["reason"])
	}
}

func TestMaxAttemptsDefaultsToOne(t *testing.T) {
	options := RunOptions{}
	capability := Capability{Limits: CapabilityLimits{MaxAttempts: 3}}
	if got := maxAttemptsForRun(options, capability); got != 1 {
		t.Fatalf("max attempts = %d, want 1", got)
	}
}

func TestMockProviderCanFailDeterministically(t *testing.T) {
	provider := MockProvider{}
	_, err := provider.Run(context.Background(), GenerationIntent{
		OperationType:  "text_generation",
		PromptTemplate: "fail this",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params: map[string]any{
			"mock_fail": true,
		},
	})
	if !errors.Is(err, ErrProviderExecution) {
		t.Fatalf("error = %v, want ErrProviderExecution", err)
	}
	if code := errorCodeForRun(err); code != "provider_error" {
		t.Fatalf("code = %q, want provider_error", code)
	}
}
