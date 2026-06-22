package worker

import (
	"errors"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidConfig = errors.New("invalid worker config")
	ErrInvalidInput  = errors.New("invalid worker input")
)

type RunTaskInput struct {
	Task db.AgentTask
}

type GenerationInput struct {
	Mode              string         `json:"mode"`
	ShotID            string         `json:"shot_id"`
	ShotClientKey     string         `json:"shot_client_key,omitempty"`
	CraftsmanThreadID string         `json:"craftsman_thread_id"`
	CraftsmanTaskID   string         `json:"craftsman_task_id"`
	Strategy          string         `json:"strategy"`
	Prompt            string         `json:"prompt"`
	NegativePrompt    string         `json:"negative_prompt,omitempty"`
	InputNodeRefs     []string       `json:"input_node_refs,omitempty"`
	TargetNodeID      string         `json:"target_node_id,omitempty"`
	Model             ModelSpec      `json:"model,omitempty"`
	Params            map[string]any `json:"params,omitempty"`
	MaxAttempts       int            `json:"max_attempts"`
}

type ModelSpec struct {
	Provider string `json:"provider,omitempty"`
	ModelID  string `json:"model_id,omitempty"`
}

type GenerationOutput struct {
	Status            string `json:"status"`
	NodeID            string `json:"node_id"`
	GenerationJobID   string `json:"generation_job_id"`
	ArtifactVersionID string `json:"artifact_version_id"`
	OperationType     string `json:"operation_type"`
}
