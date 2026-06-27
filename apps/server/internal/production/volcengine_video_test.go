package production

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

type fakeVideoTaskClient struct {
	createReq model.CreateContentGenerationTaskRequest
	createErr error
	taskID    string
	polls     []model.GetContentGenerationTaskResponse
	pollErr   error
}

type fakeProviderInputResolver struct {
	called bool
	url    string
}

func (r *fakeProviderInputResolver) ResolveInputRefs(_ context.Context, _ ProductionJob, intent GenerationIntent) (GenerationIntent, error) {
	r.called = true
	intent.InputRefs[0].StorageURL = r.url
	return intent, nil
}

func (f *fakeVideoTaskClient) CreateTask(_ context.Context, request model.CreateContentGenerationTaskRequest) (model.CreateContentGenerationTaskResponse, error) {
	f.createReq = request
	if f.createErr != nil {
		return model.CreateContentGenerationTaskResponse{}, f.createErr
	}
	if f.taskID == "" {
		f.taskID = "task-1"
	}
	return model.CreateContentGenerationTaskResponse{ID: f.taskID}, nil
}

func (f *fakeVideoTaskClient) GetTask(context.Context, string) (model.GetContentGenerationTaskResponse, error) {
	if f.pollErr != nil {
		return model.GetContentGenerationTaskResponse{}, f.pollErr
	}
	if len(f.polls) == 0 {
		return model.GetContentGenerationTaskResponse{ID: f.taskID, Status: model.StatusRunning}, nil
	}
	next := f.polls[0]
	f.polls = f.polls[1:]
	return next, nil
}

func TestVideoRuntimeCreatesPollsAndReturnsURLForSandboxPersistence(t *testing.T) {
	client := &fakeVideoTaskClient{
		taskID: "task-video-1",
		polls: []model.GetContentGenerationTaskResponse{
			{ID: "task-video-1", Status: model.StatusQueued},
			{ID: "task-video-1", Status: model.StatusRunning},
			{ID: "task-video-1", Status: model.StatusSucceeded, Content: model.Content{VideoURL: "https://provider.invalid/video.mp4"}},
		},
	}
	runtime := newVolcengineVideoRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", VideoModel: "doubao-seedance-1-0-pro-fast-251015"},
		client,
		nil,
		time.Millisecond,
		time.Second,
	)

	output := runVideoRuntime(t, runtime, videoIntent())
	if output.AssetSourceURL != "https://provider.invalid/video.mp4" || len(output.AssetContent) != 0 {
		t.Fatalf("asset url/content = %q/%d", output.AssetSourceURL, len(output.AssetContent))
	}
	if output.AssetMIME != "video/mp4" {
		t.Fatalf("asset mime = %q", output.AssetMIME)
	}
	if output.AssetMetadata["task_id"] != "task-video-1" {
		t.Fatalf("metadata = %#v", output.AssetMetadata)
	}
	if output.RequestSummary["prompt"] != "A slow camera move through a quiet neon studio." {
		t.Fatalf("request summary prompt = %#v", output.RequestSummary["prompt"])
	}
	if client.createReq.Model != "doubao-seedance-1-0-pro-fast-251015" {
		t.Fatalf("model = %q", client.createReq.Model)
	}
	if client.createReq.Duration == nil || *client.createReq.Duration != 5 {
		t.Fatalf("duration = %#v", client.createReq.Duration)
	}
	if len(client.createReq.Content) != 2 || client.createReq.Content[1].ImageURL.URL != "https://assets.example/input.png" {
		t.Fatalf("content = %#v", client.createReq.Content)
	}
}

func TestVideoRuntimeResolvesImageRefsBeforeCreatingTask(t *testing.T) {
	client := &fakeVideoTaskClient{
		taskID: "task-video-resolved",
		polls: []model.GetContentGenerationTaskResponse{{
			ID:      "task-video-resolved",
			Status:  model.StatusSucceeded,
			Content: model.Content{VideoURL: "https://provider.invalid/video.mp4"},
		}},
	}
	resolver := &fakeProviderInputResolver{url: "https://clip-anvil-temp-bucket.tos-cn-beijing.volces.com/provider-inputs/test.png?X-Tos-Signature=test"}
	runtime := newVolcengineVideoRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", VideoModel: "doubao-seedance-1-0-pro-fast-251015"},
		client,
		nil,
		time.Millisecond,
		time.Second,
	)
	runtime.inputResolver = resolver

	_ = runVideoRuntime(t, runtime, videoIntent())
	if !resolver.called {
		t.Fatal("expected resolver to be called")
	}
	if client.createReq.Content[1].ImageURL.URL != resolver.url {
		t.Fatalf("image url = %q", client.createReq.Content[1].ImageURL.URL)
	}
}

func TestVideoRuntimeOverridesResolutionWhenConfigured(t *testing.T) {
	client := &fakeVideoTaskClient{
		taskID: "task-video-resolution-override",
		polls: []model.GetContentGenerationTaskResponse{{
			ID:      "task-video-resolution-override",
			Status:  model.StatusSucceeded,
			Content: model.Content{VideoURL: "https://provider.invalid/video.mp4"},
		}},
	}
	runtime := newVolcengineVideoRuntimeForTest(
		VolcengineProviderConfig{
			APIKey:                  "test-key",
			VideoModel:              "doubao-seedance-2-0-260128",
			VideoResolutionOverride: "480p",
		},
		client,
		nil,
		time.Millisecond,
		time.Second,
	)
	intent := videoIntent()
	intent.Model.ModelID = "doubao-seedance-2-0-260128"
	intent.Params["resolution"] = "1080p"

	_ = runVideoRuntime(t, runtime, intent)
	if client.createReq.Resolution == nil || *client.createReq.Resolution != "480p" {
		t.Fatalf("resolution = %#v, want 480p", client.createReq.Resolution)
	}
}

func TestVideoRuntimeFailsForPrivateImageRefWithoutResolver(t *testing.T) {
	client := &fakeVideoTaskClient{taskID: "task-private-image"}
	runtime := newVolcengineVideoRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", VideoModel: "doubao-seedance-1-0-pro-fast-251015"},
		client,
		nil,
		time.Millisecond,
		time.Second,
	)
	intent := videoIntent()
	intent.InputRefs[0].StorageURL = "workspace-a/input.png"

	stream, err := runtime.Start(context.Background(), ProductionJob{}, intent)
	if err != nil {
		t.Fatal(err)
	}
	var failed error
	for event := range stream {
		if event.Type == ProductionEventJobFailed {
			failed = event.Err
		}
	}
	if !errors.Is(failed, ErrProviderConfig) {
		t.Fatalf("failed = %v", failed)
	}
	if client.createReq.Model != "" {
		t.Fatalf("provider task should not be created, got model %q", client.createReq.Model)
	}
}

func TestVideoRuntimeEmitsFailureForProviderFailedStatus(t *testing.T) {
	client := &fakeVideoTaskClient{
		taskID: "task-video-2",
		polls: []model.GetContentGenerationTaskResponse{{
			ID:     "task-video-2",
			Status: model.StatusFailed,
			Error:  &model.ContentGenerationError{Code: "BadPrompt", Message: "prompt rejected"},
		}},
	}
	runtime := newVolcengineVideoRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", VideoModel: "doubao-seedance-1-0-pro-fast-251015"},
		client,
		nil,
		time.Millisecond,
		time.Second,
	)
	stream, err := runtime.Start(context.Background(), ProductionJob{}, videoIntent())
	if err != nil {
		t.Fatal(err)
	}
	var failed error
	for event := range stream {
		if event.Type == ProductionEventJobFailed {
			failed = event.Err
		}
	}
	if !errors.Is(failed, ErrProviderExecution) {
		t.Fatalf("failed = %v", failed)
	}
}

func TestVideoRuntimeTimesOutPolling(t *testing.T) {
	client := &fakeVideoTaskClient{taskID: "task-video-3"}
	runtime := newVolcengineVideoRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", VideoModel: "doubao-seedance-1-0-pro-fast-251015"},
		client,
		nil,
		time.Millisecond,
		2*time.Millisecond,
	)
	stream, err := runtime.Start(context.Background(), ProductionJob{}, videoIntent())
	if err != nil {
		t.Fatal(err)
	}
	var failed error
	for event := range stream {
		if event.Type == ProductionEventJobFailed {
			failed = event.Err
		}
	}
	if !errors.Is(failed, ErrProviderExecution) {
		t.Fatalf("failed = %v", failed)
	}
}

func runVideoRuntime(t *testing.T, runtime VolcengineVideoRuntime, intent GenerationIntent) ProductionOutput {
	t.Helper()
	stream, err := runtime.Start(context.Background(), ProductionJob{}, intent)
	if err != nil {
		t.Fatal(err)
	}
	var output ProductionOutput
	for event := range stream {
		if event.Type == ProductionEventJobFailed {
			t.Fatalf("unexpected failure: %v", event.Err)
		}
		if event.Type == ProductionEventJobSucceeded {
			output = event.Output
		}
	}
	if len(output.AssetContent) == 0 {
		if output.AssetSourceURL == "" {
			t.Fatal("expected asset content or source url")
		}
	}
	return output
}

func videoIntent() GenerationIntent {
	return GenerationIntent{
		OperationType:  "video_generation",
		PromptTemplate: "A slow camera move through a quiet neon studio.",
		OutputType:     "video",
		InputRefs: []InputRef{{
			NodeType:   "image",
			StorageURL: "https://assets.example/input.png",
		}},
		Model: ModelSpec{Provider: "volcengine", ModelID: "doubao-seedance-1-0-pro-fast-251015"},
		Params: map[string]any{
			"duration_sec": 5,
			"ratio":        "16:9",
			"resolution":   "720p",
		},
	}
}
