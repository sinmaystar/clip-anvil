package modelselection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrUnsupportedProducerModel     = errors.New("unsupported producer model")
	ErrUnsupportedReasoningEffort   = errors.New("unsupported reasoning effort")
	ErrInvalidSelection             = errors.New("invalid agent model selection")
	supportedReasoningEffortOptions = []string{"minimal", "low", "medium", "high"}
)

type CapabilityStore interface {
	ListEnabledModelCapabilities(ctx context.Context) ([]db.ModelCapability, error)
}

type ModelRef struct {
	ProviderID      string `json:"provider_id"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type Selection struct {
	Producer ModelRef `json:"producer"`
}

type Option struct {
	ProviderID             string         `json:"provider_id"`
	ModelID                string         `json:"model_id"`
	DisplayName            string         `json:"display_name"`
	Limits                 map[string]any `json:"limits"`
	Pricing                map[string]any `json:"pricing"`
	SupportsThinking       bool           `json:"supports_thinking"`
	ReasoningEfforts       []string       `json:"reasoning_efforts"`
	DefaultReasoningEffort string         `json:"default_reasoning_effort"`
	MaxCompletionTokens    int            `json:"max_completion_tokens,omitempty"`
}

type Defaults struct {
	ProducerProviderID string
	ProducerModelID    string
}

type Resolved struct {
	Selection Selection `json:"selection"`
	Defaults  Selection `json:"defaults"`
	Options   []Option  `json:"options"`
}

type Service struct {
	store    CapabilityStore
	defaults Defaults
}

func NewService(store CapabilityStore, defaults Defaults) *Service {
	return &Service{store: store, defaults: defaults}
}

func (s *Service) Resolve(ctx context.Context, workspace db.Workspace) (Resolved, error) {
	options, err := s.Options(ctx)
	if err != nil {
		return Resolved{}, err
	}
	defaultSelection := s.defaultSelectionForOptions(options)
	selection := selectionFromSettings(workspace.Settings)
	if selection.Producer.ProviderID == "" || selection.Producer.ModelID == "" {
		selection = defaultSelection
	}
	option, err := findProducerOptionInOptions(options, selection.Producer)
	if err != nil && isFallbackSelectionError(err) {
		selection = defaultSelection
		option, err = findProducerOptionInOptions(options, selection.Producer)
	}
	if err != nil {
		return Resolved{}, err
	}
	if strings.TrimSpace(selection.Producer.ReasoningEffort) == "" && option.DefaultReasoningEffort != "" {
		selection.Producer.ReasoningEffort = option.DefaultReasoningEffort
	}
	return Resolved{
		Selection: selection,
		Defaults:  defaultSelection,
		Options:   options,
	}, nil
}

func (s *Service) Options(ctx context.Context) ([]Option, error) {
	rows, err := s.store.ListEnabledModelCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	out := []Option{}
	for _, row := range rows {
		if !isProducerCapability(row) {
			continue
		}
		limits := jsonObject(row.Limits)
		defaults := jsonObject(row.Defaults)
		reasoningEfforts := reasoningEffortsFromLimits(limits)
		out = append(out, Option{
			ProviderID:             row.ProviderID,
			ModelID:                row.ModelID,
			DisplayName:            row.DisplayName,
			Limits:                 limits,
			Pricing:                jsonObject(row.Pricing),
			SupportsThinking:       len(reasoningEfforts) > 0,
			ReasoningEfforts:       reasoningEfforts,
			DefaultReasoningEffort: defaultReasoningEffort(defaults, reasoningEfforts),
			MaxCompletionTokens:    intFromMap(defaults, "max_completion_tokens"),
		})
	}
	return out, nil
}

func (s *Service) ValidateProducerModel(ctx context.Context, model ModelRef) (Option, error) {
	return s.findProducerOption(ctx, model)
}

func (s *Service) ResolveProducerModel(ctx context.Context, workspace db.Workspace) (Option, error) {
	resolved, err := s.Resolve(ctx, workspace)
	if err != nil {
		return Option{}, err
	}
	option, err := s.findProducerOption(ctx, resolved.Selection.Producer)
	if err != nil {
		return Option{}, err
	}
	option.DefaultReasoningEffort = resolved.Selection.Producer.ReasoningEffort
	return option, nil
}

func (s *Service) defaultSelection() Selection {
	return Selection{Producer: ModelRef{
		ProviderID: s.defaults.ProducerProviderID,
		ModelID:    s.defaults.ProducerModelID,
	}}
}

func (s *Service) defaultSelectionForOptions(options []Option) Selection {
	configured := s.defaultSelection()
	for _, option := range options {
		if option.ProviderID == configured.Producer.ProviderID && option.ModelID == configured.Producer.ModelID {
			return configured
		}
	}
	if len(options) > 0 {
		return Selection{Producer: ModelRef{
			ProviderID: options[0].ProviderID,
			ModelID:    options[0].ModelID,
		}}
	}
	return configured
}

func (s *Service) findProducerOption(ctx context.Context, model ModelRef) (Option, error) {
	if strings.TrimSpace(model.ProviderID) == "" || strings.TrimSpace(model.ModelID) == "" {
		return Option{}, ErrInvalidSelection
	}
	options, err := s.Options(ctx)
	if err != nil {
		return Option{}, err
	}
	return findProducerOptionInOptions(options, model)
}

func findProducerOptionInOptions(options []Option, model ModelRef) (Option, error) {
	if strings.TrimSpace(model.ProviderID) == "" || strings.TrimSpace(model.ModelID) == "" {
		return Option{}, ErrInvalidSelection
	}
	for _, option := range options {
		if option.ProviderID == model.ProviderID && option.ModelID == model.ModelID {
			if err := validateReasoningEffort(option, model.ReasoningEffort); err != nil {
				return Option{}, err
			}
			return option, nil
		}
	}
	return Option{}, ErrUnsupportedProducerModel
}

func isFallbackSelectionError(err error) bool {
	return errors.Is(err, ErrInvalidSelection) ||
		errors.Is(err, ErrUnsupportedProducerModel) ||
		errors.Is(err, ErrUnsupportedReasoningEffort)
}

func ApplyToWorkspaceSettings(raw []byte, selection Selection) ([]byte, error) {
	if strings.TrimSpace(selection.Producer.ProviderID) == "" || strings.TrimSpace(selection.Producer.ModelID) == "" {
		return nil, ErrInvalidSelection
	}
	settings := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return nil, err
		}
	}
	agent := objectAt(settings, "agent")
	modelSelection := objectAt(agent, "model_selection")
	producer := map[string]any{
		"provider_id": selection.Producer.ProviderID,
		"model_id":    selection.Producer.ModelID,
	}
	if strings.TrimSpace(selection.Producer.ReasoningEffort) != "" {
		producer["reasoning_effort"] = strings.TrimSpace(selection.Producer.ReasoningEffort)
	}
	modelSelection["producer"] = producer
	agent["model_selection"] = modelSelection
	settings["agent"] = agent
	return json.Marshal(settings)
}

func selectionFromSettings(raw []byte) Selection {
	var parsed struct {
		Agent struct {
			ModelSelection Selection `json:"model_selection"`
		} `json:"agent"`
	}
	if len(raw) == 0 {
		return Selection{}
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Selection{}
	}
	return parsed.Agent.ModelSelection
}

func isProducerCapability(row db.ModelCapability) bool {
	return row.Enabled &&
		isAgentExecutableProvider(row.ProviderID) &&
		containsString(jsonStringList(row.OutputTypes), "text") &&
		containsString(jsonStringList(row.SupportedOperations), "text_generation")
}

func isAgentExecutableProvider(providerID string) bool {
	return providerID == "volcengine"
}

func jsonStringList(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func reasoningEffortsFromLimits(limits map[string]any) []string {
	raw, ok := limits["reasoning_efforts"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if containsString(supportedReasoningEffortOptions, value) {
			out = append(out, value)
		}
	}
	return out
}

func defaultReasoningEffort(defaults map[string]any, supported []string) string {
	value, _ := defaults["reasoning_effort"].(string)
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if containsString(supported, value) {
		return value
	}
	return ""
}

func validateReasoningEffort(option Option, effort string) error {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return nil
	}
	if !containsString(supportedReasoningEffortOptions, effort) {
		return ErrUnsupportedReasoningEffort
	}
	if !containsString(option.ReasoningEfforts, effort) {
		return ErrUnsupportedReasoningEffort
	}
	return nil
}

func intFromMap(values map[string]any, key string) int {
	raw, ok := values[key]
	if !ok {
		return 0
	}
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func jsonObject(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func objectAt(parent map[string]any, key string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	out := map[string]any{}
	parent[key] = out
	return out
}

func WorkspaceSettingsUpdateParams(workspaceID pgtype.UUID, settings []byte) db.UpdateWorkspaceSettingsParams {
	return db.UpdateWorkspaceSettingsParams{ID: workspaceID, Settings: settings}
}
