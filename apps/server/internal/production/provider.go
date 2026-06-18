package production

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrProviderConfig      = errors.New("provider configuration error")
	ErrProviderExecution   = errors.New("provider execution error")
)

type ProviderConfig struct {
	ProviderMode     string
	DefaultProvider  string
	DefaultTextModel string
	Volcengine       VolcengineProviderConfig
}

type VolcengineProviderConfig struct {
	APIKey     string
	BaseURL    string
	TextModel  string
	ImageModel string
	VideoModel string
}

type ProviderRegistry struct {
	cfg       ProviderConfig
	providers map[string]ProviderBridge
}

func NewProviderRegistry(cfg ProviderConfig) *ProviderRegistry {
	if strings.TrimSpace(cfg.ProviderMode) == "" {
		cfg.ProviderMode = "mock"
	}
	if strings.TrimSpace(cfg.DefaultProvider) == "" {
		cfg.DefaultProvider = "mock"
	}
	if strings.TrimSpace(cfg.DefaultTextModel) == "" {
		cfg.DefaultTextModel = "mock-text"
	}
	return &ProviderRegistry{
		cfg: cfg,
		providers: map[string]ProviderBridge{
			"mock":            MockProvider{},
			"volcengine":      NewVolcengineProvider(cfg.Volcengine),
			"internal_ffmpeg": NewInternalFFmpegProvider(nil),
		},
	}
}

func (r *ProviderRegistry) ApplyDefaults(intent GenerationIntent) GenerationIntent {
	if strings.TrimSpace(intent.Model.Provider) == "" {
		intent.Model.Provider = r.cfg.DefaultProvider
	}
	if strings.TrimSpace(intent.Model.ModelID) == "" {
		intent.Model.ModelID = defaultModelForOutput(intent.OutputType, r.cfg)
	}
	if intent.Params == nil {
		intent.Params = map[string]any{}
	}
	return intent
}

func (r *ProviderRegistry) Resolve(intent GenerationIntent) (ProviderBridge, error) {
	providerID := strings.TrimSpace(intent.Model.Provider)
	if providerID == "" {
		providerID = r.cfg.DefaultProvider
	}
	provider, ok := r.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderUnavailable, providerID)
	}
	return provider, nil
}

func (r *ProviderRegistry) Register(providerID string, provider ProviderBridge) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || provider == nil {
		return
	}
	r.providers[providerID] = provider
}

func defaultModelForOutput(outputType string, cfg ProviderConfig) string {
	if cfg.ProviderMode == "real" && cfg.DefaultProvider == "volcengine" {
		switch outputType {
		case "image":
			return cfg.Volcengine.ImageModel
		case "video":
			return cfg.Volcengine.VideoModel
		default:
			return cfg.Volcengine.TextModel
		}
	}
	return cfg.DefaultTextModel
}
