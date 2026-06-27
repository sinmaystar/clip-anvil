package production

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	cgmodel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

type volcengineVideoTaskClient interface {
	CreateTask(ctx context.Context, request cgmodel.CreateContentGenerationTaskRequest) (cgmodel.CreateContentGenerationTaskResponse, error)
	GetTask(ctx context.Context, taskID string) (cgmodel.GetContentGenerationTaskResponse, error)
}

type volcengineVideoTaskClientFactory func(cfg VolcengineProviderConfig, httpClient *http.Client) (volcengineVideoTaskClient, error)

type arkVideoTaskClient struct {
	client *arkruntime.Client
}

func newArkVideoTaskClient(cfg VolcengineProviderConfig, httpClient *http.Client) (volcengineVideoTaskClient, error) {
	options := []arkruntime.ConfigOption{
		arkruntime.WithBaseUrl(strings.TrimSpace(cfg.BaseURL)),
		arkruntime.WithRegion(strings.TrimSpace(cfg.Region)),
		arkruntime.WithTimeout(10 * time.Minute),
	}
	if httpClient != nil {
		options = append(options, arkruntime.WithHTTPClient(httpClient))
	}
	return arkVideoTaskClient{client: arkruntime.NewClientWithApiKey(cfg.APIKey, options...)}, nil
}

func (c arkVideoTaskClient) CreateTask(ctx context.Context, request cgmodel.CreateContentGenerationTaskRequest) (cgmodel.CreateContentGenerationTaskResponse, error) {
	return c.client.CreateContentGenerationTask(ctx, request)
}

func (c arkVideoTaskClient) GetTask(ctx context.Context, taskID string) (cgmodel.GetContentGenerationTaskResponse, error) {
	return c.client.GetContentGenerationTask(ctx, cgmodel.GetContentGenerationTaskRequest{ID: taskID})
}

type VolcengineVideoRuntime struct {
	cfg           VolcengineProviderConfig
	factory       volcengineVideoTaskClientFactory
	httpClient    *http.Client
	inputResolver ProviderInputResolver
	pollInterval  time.Duration
	maxPoll       time.Duration
}

func NewVolcengineVideoRuntime(cfg VolcengineProviderConfig, httpClient *http.Client, pollInterval time.Duration, maxPoll time.Duration) VolcengineVideoRuntime {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if maxPoll <= 0 {
		maxPoll = 20 * time.Minute
	}
	return VolcengineVideoRuntime{
		cfg:          cfg,
		factory:      newArkVideoTaskClient,
		httpClient:   httpClient,
		pollInterval: pollInterval,
		maxPoll:      maxPoll,
	}
}

func newVolcengineVideoRuntimeForTest(cfg VolcengineProviderConfig, client volcengineVideoTaskClient, httpClient *http.Client, pollInterval time.Duration, maxPoll time.Duration) VolcengineVideoRuntime {
	if pollInterval <= 0 {
		pollInterval = time.Millisecond
	}
	if maxPoll <= 0 {
		maxPoll = time.Second
	}
	return VolcengineVideoRuntime{
		cfg: cfg,
		factory: func(VolcengineProviderConfig, *http.Client) (volcengineVideoTaskClient, error) {
			return client, nil
		},
		httpClient:   httpClient,
		pollInterval: pollInterval,
		maxPoll:      maxPoll,
	}
}

func (r VolcengineVideoRuntime) Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error) {
	if strings.TrimSpace(r.cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for provider volcengine", ErrProviderConfig)
	}
	modelID := strings.TrimSpace(intent.Model.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(r.cfg.VideoModel)
	}
	if modelID == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_VIDEO_MODEL is required", ErrProviderConfig)
	}
	client, err := r.factory(r.cfg, r.httpClient)
	if err != nil {
		return nil, fmt.Errorf("%w: create ark content generation client: %v", ErrProviderUnavailable, err)
	}
	events := make(chan ProductionEvent, 16)
	go r.generate(ctx, client, modelID, job, intent, events)
	return events, nil
}

func (r VolcengineVideoRuntime) generate(ctx context.Context, client volcengineVideoTaskClient, modelID string, job ProductionJob, intent GenerationIntent, events chan<- ProductionEvent) {
	defer close(events)
	rendered := strings.TrimSpace(intent.EffectivePrompt())
	if rendered == "" {
		rendered = "empty prompt"
	}
	if r.inputResolver != nil {
		resolved, err := r.inputResolver.ResolveInputRefs(ctx, job, intent)
		if err != nil {
			events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
			return
		}
		intent = resolved
	}
	if err := validateProviderReachableVideoInputs(intent); err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
		return
	}
	request := videoTaskRequest(modelID, rendered, intent)
	if resolution := strings.TrimSpace(r.cfg.VideoResolutionOverride); resolution != "" {
		request.Resolution = &resolution
	}
	created, err := client.CreateTask(ctx, request)
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: create ark video task: %v", ErrProviderExecution, err)}
		return
	}
	taskID := strings.TrimSpace(created.ID)
	if taskID == "" {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: ark video task returned empty id", ErrProviderExecution)}
		return
	}
	events <- ProductionEvent{
		JobID:        job.ID,
		WorkspaceID:  job.WorkspaceID,
		TargetNodeID: job.TargetNodeID,
		Type:         ProductionEventProviderTaskCreated,
		Progress:     10,
		Payload:      map[string]any{"provider": "volcengine", "task_id": taskID},
	}

	started := time.Now()
	attempt := 0
	for {
		if r.maxPoll > 0 && time.Since(started) > r.maxPoll {
			events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: ark video task %s timed out", ErrProviderExecution, taskID)}
			return
		}
		select {
		case <-ctx.Done():
			events <- ProductionEvent{Type: ProductionEventJobCancelled, Progress: 100, Err: ctx.Err()}
			return
		default:
		}
		task, err := client.GetTask(ctx, taskID)
		if err != nil {
			events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: poll ark video task %s: %v", ErrProviderExecution, taskID, err)}
			return
		}
		switch task.Status {
		case cgmodel.StatusSucceeded:
			r.emitVideoSuccess(ctx, job, intent, rendered, request, task, events)
			return
		case cgmodel.StatusFailed:
			events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: ark video task %s failed: %s", ErrProviderExecution, taskID, contentGenerationErrorMessage(task.Error))}
			return
		case cgmodel.StatusCancelled:
			events <- ProductionEvent{Type: ProductionEventJobCancelled, Progress: 100, Payload: map[string]any{"task_id": taskID, "status": task.Status}}
			return
		default:
			attempt++
			progress := int32(20 + attempt*5)
			if progress > 90 {
				progress = 90
			}
			events <- ProductionEvent{
				JobID:        job.ID,
				WorkspaceID:  job.WorkspaceID,
				TargetNodeID: job.TargetNodeID,
				Type:         ProductionEventProviderProgress,
				Progress:     progress,
				Payload:      map[string]any{"provider": "volcengine", "task_id": taskID, "status": task.Status},
			}
			timer := time.NewTimer(r.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				events <- ProductionEvent{Type: ProductionEventJobCancelled, Progress: 100, Err: ctx.Err()}
				return
			case <-timer.C:
			}
		}
	}
}

func validateProviderReachableVideoInputs(intent GenerationIntent) error {
	for _, ref := range intent.InputRefs {
		if ref.NodeType != "image" || strings.TrimSpace(ref.StorageURL) == "" {
			continue
		}
		if strings.HasPrefix(ref.StorageURL, "http://") || strings.HasPrefix(ref.StorageURL, "https://") {
			continue
		}
		return fmt.Errorf("%w: image input %s must be staged to a provider-reachable URL before video generation", ErrProviderConfig, uuidToString(ref.NodeID))
	}
	return nil
}

func (r VolcengineVideoRuntime) emitVideoSuccess(ctx context.Context, job ProductionJob, intent GenerationIntent, rendered string, request cgmodel.CreateContentGenerationTaskRequest, task cgmodel.GetContentGenerationTaskResponse, events chan<- ProductionEvent) {
	videoURL := strings.TrimSpace(task.Content.VideoURL)
	if videoURL == "" {
		videoURL = strings.TrimSpace(task.Content.FileURL)
	}
	if videoURL == "" {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: ark video task %s returned no video url", ErrProviderExecution, task.ID)}
		return
	}
	events <- ProductionEvent{
		JobID:        job.ID,
		WorkspaceID:  job.WorkspaceID,
		TargetNodeID: job.TargetNodeID,
		Type:         ProductionEventAssetDownloading,
		Progress:     95,
		Payload:      map[string]any{"provider": "volcengine", "task_id": task.ID},
	}
	events <- ProductionEvent{
		JobID:        job.ID,
		WorkspaceID:  job.WorkspaceID,
		TargetNodeID: job.TargetNodeID,
		Type:         ProductionEventJobSucceeded,
		Progress:     100,
		Output: ProductionOutput{
			RenderedPrompt: rendered,
			AssetSourceURL: videoURL,
			AssetMIME:      mimeForVideoURL(videoURL),
			AssetMetadata: map[string]any{
				"provider": "volcengine",
				"task_id":  task.ID,
				"source":   "url",
			},
			RequestSummary: map[string]any{
				"provider":       "volcengine",
				"model_id":       request.Model,
				"operation_type": intent.OperationType,
				"prompt":         rendered,
				"params":         intent.Params,
				"input_refs":     len(intent.InputRefs),
			},
			ResponseSummary: map[string]any{
				"provider":          "volcengine",
				"task_id":           task.ID,
				"status":            task.Status,
				"video_url":         videoURL,
				"revised_prompt":    stringPtrValue(task.RevisedPrompt),
				"duration":          int64PtrValue(task.Duration),
				"resolution":        stringPtrValue(task.Resolution),
				"ratio":             stringPtrValue(task.Ratio),
				"generate_audio":    boolPtrValue(task.GenerateAudio),
				"safety_identifier": stringPtrValue(task.SafetyIdentifier),
			},
		},
	}
}

func videoTaskRequest(modelID string, rendered string, intent GenerationIntent) cgmodel.CreateContentGenerationTaskRequest {
	content := []*cgmodel.CreateContentGenerationContentItem{
		{
			Type: cgmodel.ContentGenerationContentItemTypeText,
			Text: &rendered,
		},
	}
	for _, ref := range intent.InputRefs {
		url := strings.TrimSpace(ref.StorageURL)
		if url == "" {
			continue
		}
		if item := videoContentItemForInputRef(ref, url); item != nil {
			content = append(content, item)
		}
	}
	request := cgmodel.CreateContentGenerationTaskRequest{
		Model:   modelID,
		Content: content,
	}
	if duration, ok := numericParam(intent.Params, "duration_sec"); ok && duration > 0 {
		value := int64(duration)
		request.Duration = &value
	}
	if seed, ok := numericParam(intent.Params, "seed"); ok {
		value := int64(seed)
		request.Seed = &value
	}
	if ratio := firstStringParam(intent.Params, "ratio", "aspect_ratio"); ratio != "" {
		request.Ratio = &ratio
	}
	if resolution := stringParam(intent.Params, "resolution", ""); resolution != "" {
		request.Resolution = &resolution
	}
	if serviceTier := stringParam(intent.Params, "service_tier", ""); serviceTier != "" {
		request.ServiceTier = &serviceTier
	}
	if watermark, ok := optionalBoolParam(intent.Params, "watermark"); ok {
		request.Watermark = &watermark
	}
	if generateAudio, ok := optionalBoolParam(intent.Params, "generate_audio"); ok {
		request.GenerateAudio = &generateAudio
	}
	if cameraFixed, ok := optionalBoolParam(intent.Params, "camera_fixed"); ok {
		request.CameraFixed = &cameraFixed
	}
	return request
}

func videoContentItemForInputRef(ref InputRef, url string) *cgmodel.CreateContentGenerationContentItem {
	item := &cgmodel.CreateContentGenerationContentItem{}
	if role := strings.TrimSpace(ref.ModelRole); role != "" {
		item.Role = &role
	}
	switch contentTypeForVideoInputRef(ref) {
	case "image_url":
		item.Type = cgmodel.ContentGenerationContentItemTypeImage
		item.ImageURL = &cgmodel.ImageURL{URL: url}
	case "video_url":
		item.Type = cgmodel.ContentGenerationContentItemTypeVideo
		item.VideoURL = &cgmodel.VideoUrl{Url: url}
	case "audio_url":
		item.Type = cgmodel.ContentGenerationContentItemTypeAudio
		item.AudioURL = &cgmodel.AudioUrl{Url: url}
	default:
		return nil
	}
	return item
}

func contentTypeForVideoInputRef(ref InputRef) string {
	if contentType := strings.TrimSpace(ref.ContentType); contentType != "" {
		return contentType
	}
	switch strings.TrimSpace(ref.NodeType) {
	case "image":
		return "image_url"
	case "video":
		return "video_url"
	case "audio":
		return "audio_url"
	default:
		return ""
	}
}

func mimeForVideoURL(rawURL string) string {
	path := strings.ToLower(strings.TrimSpace(rawURL))
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}
	switch {
	case strings.HasSuffix(path, ".webm"):
		return "video/webm"
	case strings.HasSuffix(path, ".mov"):
		return "video/quicktime"
	default:
		return "video/mp4"
	}
}

func optionalBoolParam(params map[string]any, key string) (bool, bool) {
	value, ok := params[key]
	if !ok {
		return false, false
	}
	flag, ok := value.(bool)
	return flag, ok
}

func firstStringParam(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringParam(params, key, ""); value != "" {
			return value
		}
	}
	return ""
}

func contentGenerationErrorMessage(err *cgmodel.ContentGenerationError) string {
	if err == nil {
		return "unknown error"
	}
	if strings.TrimSpace(err.Code) == "" {
		return err.Message
	}
	if strings.TrimSpace(err.Message) == "" {
		return err.Code
	}
	return err.Code + ": " + err.Message
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int64PtrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func boolPtrValue(value *bool) bool {
	return value != nil && *value
}
