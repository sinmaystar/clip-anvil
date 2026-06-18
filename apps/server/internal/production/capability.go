package production

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var ErrCapabilityMismatch = errors.New("capability mismatch")

type Capability struct {
	ProviderID              string
	ModelID                 string
	OutputTypes             []string
	SupportedOperations     []string
	SupportedInputNodeTypes []string
	Limits                  CapabilityLimits
}

type CapabilityLimits struct {
	MaxPromptChars   int
	MaxAttempts      int
	AllowedDurations []int
}

func CapabilityFromRow(row db.ModelCapability) (Capability, error) {
	outputTypes, err := stringList(row.OutputTypes)
	if err != nil {
		return Capability{}, err
	}
	operations, err := stringList(row.SupportedOperations)
	if err != nil {
		return Capability{}, err
	}
	inputTypes, err := stringList(row.SupportedInputNodeTypes)
	if err != nil {
		return Capability{}, err
	}
	limits, err := capabilityLimits(row.Limits)
	if err != nil {
		return Capability{}, err
	}
	return Capability{
		ProviderID:              row.ProviderID,
		ModelID:                 row.ModelID,
		OutputTypes:             outputTypes,
		SupportedOperations:     operations,
		SupportedInputNodeTypes: inputTypes,
		Limits:                  limits,
	}, nil
}

func ValidateCapability(intent GenerationIntent, capability Capability) error {
	if !contains(capability.OutputTypes, intent.OutputType) {
		return fmt.Errorf("%w: model %s/%s does not support output type %s", ErrCapabilityMismatch, capability.ProviderID, capability.ModelID, intent.OutputType)
	}
	if !contains(capability.SupportedOperations, intent.OperationType) {
		return fmt.Errorf("%w: model %s/%s does not support operation %s", ErrCapabilityMismatch, capability.ProviderID, capability.ModelID, intent.OperationType)
	}
	if capability.Limits.MaxPromptChars > 0 && len([]rune(intent.PromptTemplate)) > capability.Limits.MaxPromptChars {
		return fmt.Errorf("%w: prompt exceeds max_prompt_chars %d", ErrCapabilityMismatch, capability.Limits.MaxPromptChars)
	}
	if len(capability.Limits.AllowedDurations) > 0 {
		duration, ok := numericParam(intent.Params, "duration_sec")
		if ok && !containsInt(capability.Limits.AllowedDurations, int(duration)) {
			return fmt.Errorf("%w: duration_sec %d is not supported", ErrCapabilityMismatch, int(duration))
		}
	}
	return nil
}

func stringList(raw []byte) ([]string, error) {
	var values []string
	if len(raw) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func capabilityLimits(raw []byte) (CapabilityLimits, error) {
	var payload struct {
		MaxPromptChars   int   `json:"max_prompt_chars"`
		MaxAttempts      int   `json:"max_attempts"`
		AllowedDurations []int `json:"durations_sec"`
	}
	if len(raw) == 0 {
		return CapabilityLimits{}, nil
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return CapabilityLimits{}, err
	}
	return CapabilityLimits{
		MaxPromptChars:   payload.MaxPromptChars,
		MaxAttempts:      payload.MaxAttempts,
		AllowedDurations: payload.AllowedDurations,
	}, nil
}

func contains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func numericParam(params map[string]any, key string) (float64, bool) {
	value, ok := params[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}
