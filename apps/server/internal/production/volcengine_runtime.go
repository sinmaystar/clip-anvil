package production

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type VolcengineProductionRuntime struct {
	text   EinoProductionRuntime
	image  EinoProductionRuntime
	video  EinoProductionRuntime
	legacy EinoProductionRuntime
}

func NewVolcengineProductionRuntime(cfg VolcengineProviderConfig, httpClient *http.Client, pollInterval time.Duration, maxPoll time.Duration, legacy EinoProductionRuntime, inputResolver ProviderInputResolver) VolcengineProductionRuntime {
	video := NewVolcengineVideoRuntime(cfg, httpClient, pollInterval, maxPoll)
	video.inputResolver = inputResolver
	return VolcengineProductionRuntime{
		text:   NewVolcengineTextRuntime(cfg),
		image:  NewVolcengineImageRuntime(cfg, httpClient),
		video:  video,
		legacy: legacy,
	}
}

func (r VolcengineProductionRuntime) Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error) {
	if strings.TrimSpace(intent.Model.Provider) != "volcengine" {
		if r.legacy == nil {
			return nil, fmt.Errorf("%w: provider %s is not configured", ErrProviderUnavailable, intent.Model.Provider)
		}
		return r.legacy.Start(ctx, job, intent)
	}
	switch intent.OutputType {
	case "text":
		return r.text.Start(ctx, job, intent)
	case "image":
		return r.image.Start(ctx, job, intent)
	case "video":
		return r.video.Start(ctx, job, intent)
	case "audio":
		return nil, fmt.Errorf("%w: volcengine audio generation is on hold because no production audio model is configured", ErrCapabilityMismatch)
	default:
		return nil, fmt.Errorf("%w: unsupported volcengine output type %s", ErrCapabilityMismatch, intent.OutputType)
	}
}
