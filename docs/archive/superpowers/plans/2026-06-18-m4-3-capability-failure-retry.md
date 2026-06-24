# M4.3 Capability Validation, Failure Records, And Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Do not use subagents for this milestone unless the user explicitly changes that instruction.

**Goal:** Enforce backend model capability validation before provider execution, persist every invalid or failed run as a failed `generation_job`, and add bounded retry chains.

**Architecture:** Add durable `model_provider` and `model_capability` tables, then introduce a production capability validator that runs after `GenerationIntent` construction and before provider resolution/execution. Refactor the run service into attempt-aware helpers so capability failures, provider failures, manual retry, and automatic retry all persist structured jobs with `attempt`, `max_attempts`, and `parent_job_id`.

**Tech Stack:** Go 1.26, Hertz, pgx v5, sqlc v1.31, goose migrations, PostgreSQL JSONB capability records, Node smoke scripts.

---

## Spec Source

- Milestone: `docs/milestones/m4-shared-production-foundation.md`
- Parent roadmap: `docs/milestones/m3-m6-studio-agent-roadmap.md`
- Shared production design: `docs/superpowers/specs/2026-06-18-studio-agent-shared-production-design.md`
- Database design: `docs/superpowers/specs/2026-06-18-production-database-technical-design.md`
- M4.2 baseline plan: `docs/superpowers/plans/2026-06-18-m4-2-generation-intent-provider-bridge.md`

M4.3 acceptance to satisfy:

- Unsupported model/operation combinations do not call a provider.
- Capability mismatch creates a failed job with structured error code and readable message.
- Mock provider failure creates a failed job.
- Retry creates a new job linked to the previous attempt.
- Automatic retry has a hard maximum attempt count.

Non-goals for M4.3:

- Rich model selection UI.
- Full Volcengine request mapping or real external API calls.
- Reference Pack expansion, stale propagation, or internal media extraction.
- Full media output creation for image/video success paths. M4.3 can validate image/video capabilities and fail unsupported paths before provider calls; successful artifact creation remains text-only until later phases add real media adapters.

## File Structure

- Create `apps/server/migrations/008_m4_3_model_capability.sql`: provider/capability tables and mock seed data.
- Create `apps/server/sqlc/queries/model.sql`: model provider and capability queries.
- Modify generated `apps/server/internal/store/db/*.go` via `make sqlc-generate`.
- Create `apps/server/internal/production/capability.go`: capability structs, JSON decoding, validator, capability errors.
- Create `apps/server/internal/production/retry.go`: run options, max-attempt parsing, attempt helpers.
- Modify `apps/server/internal/production/provider.go`: registry gets capability validator support and provider failure error type.
- Modify `apps/server/internal/production/mock_provider.go`: support deterministic mock failures via `params.mock_fail`.
- Modify `apps/server/internal/production/service.go`: validate before provider calls, persist capability failures, provider failures, parent/attempt/max_attempts, and automatic retries.
- Modify `apps/server/internal/production/service_test.go`: unit tests for capability validation, mock provider failure, error codes, and attempt math.
- Modify `apps/server/sqlc/queries/production.sql`: job queries for retry and latest attempts.
- Modify `apps/server/internal/api/run_handler.go`: expose job attempt fields, parse run options, and add `POST /api/jobs/:id/retry`.
- Modify `apps/server/internal/api/run_handler_test.go`: response and option parsing tests.
- Modify `apps/server/cmd/server/main.go`: wire retry route.
- Create `scripts/smoke-m4-3.sh`: E2E smoke for capability mismatch, mock provider failure, retry chain, and max-attempt cap.
- Modify `docs/milestones/m4-shared-production-foundation.md`: add M4.3 completion record after implementation.

---

### Task 1: Add Model Capability Schema And Seeds

**Files:**
- Create: `apps/server/migrations/008_m4_3_model_capability.sql`

- [ ] **Step 1: Write the migration**

Create `apps/server/migrations/008_m4_3_model_capability.sql`:

```sql
-- +goose Up
CREATE TABLE model_provider (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE model_capability (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id TEXT NOT NULL REFERENCES model_provider(id) ON DELETE CASCADE,
    model_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    output_types JSONB NOT NULL DEFAULT '[]',
    supported_operations JSONB NOT NULL DEFAULT '[]',
    supported_input_node_types JSONB NOT NULL DEFAULT '[]',
    limits JSONB NOT NULL DEFAULT '{}',
    pricing JSONB NOT NULL DEFAULT '{}',
    defaults JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_model_capability UNIQUE (provider_id, model_id)
);

CREATE INDEX idx_model_capability_provider ON model_capability(provider_id);
CREATE INDEX idx_model_capability_enabled ON model_capability(provider_id, enabled);

INSERT INTO model_provider (id, display_name, provider_type, config, enabled)
VALUES
    ('mock', 'Mock Provider', 'media_generation', '{}', true),
    ('volcengine', 'Volcengine Ark', 'media_generation', '{}', true),
    ('internal_ffmpeg', 'Internal FFmpeg', 'internal_media', '{}', true);

INSERT INTO model_capability (
    provider_id,
    model_id,
    display_name,
    output_types,
    supported_operations,
    supported_input_node_types,
    limits,
    pricing,
    defaults,
    enabled
) VALUES
    (
        'mock',
        'mock-text',
        'Mock Text',
        '["text"]',
        '["text_generation"]',
        '["text"]',
        '{"max_prompt_chars": 8000, "max_attempts": 3}',
        '{"tier": "mock"}',
        '{"temperature": 0.2}',
        true
    ),
    (
        'mock',
        'mock-image-only',
        'Mock Image Only',
        '["image"]',
        '["text_to_image"]',
        '["text", "image"]',
        '{"max_prompt_chars": 8000, "max_attempts": 3}',
        '{"tier": "mock"}',
        '{}',
        true
    ),
    (
        'mock',
        'mock-video',
        'Mock Video',
        '["video"]',
        '["text_to_video", "image_to_video"]',
        '["text", "image", "video"]',
        '{"max_prompt_chars": 8000, "max_attempts": 3, "durations_sec": [4, 5, 8]}',
        '{"tier": "mock"}',
        '{"duration_sec": 5}',
        true
    ),
    (
        'internal_ffmpeg',
        'ffmpeg',
        'Internal FFmpeg',
        '["image"]',
        '["extract_first_frame", "extract_last_frame"]',
        '["video"]',
        '{"max_attempts": 1}',
        '{"tier": "internal"}',
        '{}',
        true
    );

-- +goose Down
DROP INDEX IF EXISTS idx_model_capability_enabled;
DROP INDEX IF EXISTS idx_model_capability_provider;
DROP TABLE IF EXISTS model_capability;
DROP TABLE IF EXISTS model_provider;
```

- [ ] **Step 2: Run migration**

Run:

```bash
make migrate-up
```

Expected: migration 008 applies successfully.

---

### Task 2: Add sqlc Queries For Capabilities And Retry Jobs

**Files:**
- Create: `apps/server/sqlc/queries/model.sql`
- Modify: `apps/server/sqlc/queries/production.sql`
- Regenerate: `apps/server/internal/store/db/*.go`

- [ ] **Step 1: Add model queries**

Create `apps/server/sqlc/queries/model.sql`:

```sql
-- name: GetEnabledModelProvider :one
SELECT *
FROM model_provider
WHERE id = $1
  AND enabled = true;

-- name: GetEnabledModelCapability :one
SELECT *
FROM model_capability
WHERE provider_id = $1
  AND model_id = $2
  AND enabled = true;

-- name: ListEnabledModelCapabilities :many
SELECT *
FROM model_capability
WHERE enabled = true
ORDER BY provider_id, model_id;
```

- [ ] **Step 2: Add retry job queries**

Append to `apps/server/sqlc/queries/production.sql`:

```sql
-- name: ListGenerationJobsByParent :many
SELECT *
FROM generation_job
WHERE parent_job_id = $1
ORDER BY attempt;

-- name: LatestGenerationJobInChain :one
WITH RECURSIVE chain AS (
    SELECT *
    FROM generation_job
    WHERE id = $1
    UNION ALL
    SELECT child.*
    FROM generation_job child
    JOIN chain parent ON child.parent_job_id = parent.id
)
SELECT *
FROM chain
ORDER BY attempt DESC, created_at DESC
LIMIT 1;
```

- [ ] **Step 3: Regenerate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: generated DB code includes `ModelProvider`, `ModelCapability`, `GetEnabledModelCapability`, `ListGenerationJobsByParent`, and `LatestGenerationJobInChain`.

---

### Task 3: Implement Capability Validator

**Files:**
- Create: `apps/server/internal/production/capability.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Write failing validator tests**

Add these tests to `apps/server/internal/production/service_test.go`:

```go
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
			MaxPromptChars:  100,
			MaxAttempts:     3,
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
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestCapabilityValidator' -count=1
```

Expected: FAIL because capability types and validator are not implemented.

- [ ] **Step 3: Implement capability validator**

Create `apps/server/internal/production/capability.go`:

```go
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
```

- [ ] **Step 4: Update error code mapping**

In `apps/server/internal/production/service.go`, update `errorCodeForRun`:

```go
case errors.Is(err, ErrCapabilityMismatch):
	return "capability_mismatch"
```

In `apps/server/internal/api/run_handler.go`, update `statusForRunError` so `ErrCapabilityMismatch` returns `400`.

- [ ] **Step 5: Run validator tests and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestCapabilityValidator|TestErrorCodeForProviderConfig' -count=1
```

Expected: PASS.

---

### Task 4: Load And Apply Capabilities Before Provider Calls

**Files:**
- Modify: `apps/server/internal/production/service.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Write helper tests for max attempts**

Add this test to `apps/server/internal/production/service_test.go`:

```go
func TestMaxAttemptsRespectsCapabilityLimit(t *testing.T) {
	options := RunOptions{MaxAttempts: 10}
	capability := Capability{Limits: CapabilityLimits{MaxAttempts: 3}}
	if got := maxAttemptsForRun(options, capability); got != 3 {
		t.Fatalf("max attempts = %d, want 3", got)
	}
}

func TestMaxAttemptsDefaultsToOne(t *testing.T) {
	options := RunOptions{}
	capability := Capability{Limits: CapabilityLimits{MaxAttempts: 3}}
	if got := maxAttemptsForRun(options, capability); got != 1 {
		t.Fatalf("max attempts = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestMaxAttempts' -count=1
```

Expected: FAIL because `RunOptions` and `maxAttemptsForRun` do not exist.

- [ ] **Step 3: Create retry option helpers**

Create `apps/server/internal/production/retry.go`:

```go
package production

import "github.com/jackc/pgx/v5/pgtype"

type RunOptions struct {
	MaxAttempts int
	ParentJobID pgtype.UUID
	Attempt int
}

func maxAttemptsForRun(options RunOptions, capability Capability) int32 {
	requested := options.MaxAttempts
	if requested <= 0 {
		requested = 1
	}
	if capability.Limits.MaxAttempts > 0 && requested > capability.Limits.MaxAttempts {
		requested = capability.Limits.MaxAttempts
	}
	if requested < 1 {
		requested = 1
	}
	return int32(requested)
}
```

- [ ] **Step 4: Add capability loading in service**

In `apps/server/internal/production/service.go`, add:

```go
func (s *Service) capabilityForIntent(ctx context.Context, intent GenerationIntent) (Capability, error) {
	row, err := s.queries.GetEnabledModelCapability(ctx, db.GetEnabledModelCapabilityParams{
		ProviderID: intent.Model.Provider,
		ModelID:    intent.Model.ModelID,
	})
	if err != nil {
		return Capability{}, fmt.Errorf("%w: %s/%s", ErrCapabilityMismatch, intent.Model.Provider, intent.Model.ModelID)
	}
	return CapabilityFromRow(row)
}
```

Add `fmt` to imports.

- [ ] **Step 5: Change RunNode to accept options**

Change:

```go
func (s *Service) RunNode(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy) (RunResult, error)
```

to:

```go
func (s *Service) RunNode(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy, options RunOptions) (RunResult, error)
```

Inside `RunNode`, after applying provider defaults and before provider resolve:

```go
capability, err := s.capabilityForIntent(ctx, intent)
if err != nil {
	if jobErr := s.createFailedJob(ctx, node, intent, err, nil, 1, 1); jobErr != nil {
		return RunResult{}, jobErr
	}
	return RunResult{}, err
}
if err := ValidateCapability(intent, capability); err != nil {
	if jobErr := s.createFailedJob(ctx, node, intent, err, nil, 1, maxAttemptsForRun(options, capability)); jobErr != nil {
		return RunResult{}, jobErr
	}
	return RunResult{}, err
}
```

Remove the M4.2 `node.NodeType != db.NodeTypeText` early guard so capability validation can block unsupported non-text combinations before provider calls.

- [ ] **Step 6: Update callers temporarily**

In `apps/server/internal/api/run_handler.go`, update the call to pass empty options:

```go
result, err := h.service.RunNode(ctx, nodeID, production.RequestedBy{Type: "user", ID: uuidToString(accountID)}, production.RunOptions{})
```

- [ ] **Step 7: Run tests and verify compile**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: tests compile and pass. Later tasks will add behavior tests around retry and smoke.

---

### Task 5: Add Deterministic Mock Provider Failure

**Files:**
- Modify: `apps/server/internal/production/provider.go`
- Modify: `apps/server/internal/production/mock_provider.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Write failing mock failure test**

Add this test:

```go
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
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run TestMockProviderCanFailDeterministically -count=1
```

Expected: FAIL because `ErrProviderExecution` and mock fail behavior do not exist.

- [ ] **Step 3: Add provider execution error**

In `apps/server/internal/production/provider.go`, add:

```go
ErrProviderExecution = errors.New("provider execution error")
```

- [ ] **Step 4: Add mock failure behavior**

In `apps/server/internal/production/mock_provider.go`, before building the success response:

```go
if shouldMockFail(intent.Params) {
	return ProviderResult{}, fmt.Errorf("%w: mock provider failure", ErrProviderExecution)
}
```

Add helper:

```go
func shouldMockFail(params map[string]any) bool {
	value, ok := params["mock_fail"]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}
```

- [ ] **Step 5: Run mock failure test and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run TestMockProviderCanFailDeterministically -count=1
```

Expected: PASS.

---

### Task 6: Persist Attempt-Aware Failed Jobs And Automatic Retry

**Files:**
- Modify: `apps/server/internal/production/service.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Refactor failed job helper signature**

Change:

```go
func (s *Service) createFailedJob(ctx context.Context, node db.MediaNode, intent GenerationIntent, runErr error) error
```

to:

```go
func (s *Service) createFailedJob(
	ctx context.Context,
	node db.MediaNode,
	intent GenerationIntent,
	runErr error,
	parentJobID *pgtype.UUID,
	attempt int32,
	maxAttempts int32,
) (db.GenerationJob, error)
```

Inside `CreateGenerationJobParams`, set:

```go
ParentJobID:      nullableUUID(parentJobID),
Attempt:          attempt,
MaxAttempts:      maxAttempts,
RetryPolicy:      retryPolicyJSON(maxAttempts),
ProviderResponse: providerFailureResponse(runErr),
```

Add helpers:

```go
func nullableUUID(value *pgtype.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return *value
}

func retryPolicyJSON(maxAttempts int32) []byte {
	raw, _ := json.Marshal(map[string]any{"max_attempts": maxAttempts})
	return raw
}

func providerFailureResponse(err error) []byte {
	raw, _ := json.Marshal(map[string]any{
		"error": err.Error(),
		"code":  errorCodeForRun(err),
	})
	return raw
}
```

- [ ] **Step 2: Add run attempt loop**

In `RunNode`, after capability validation and provider resolution, replace the single provider call with:

```go
maxAttempts := maxAttemptsForRun(options, capability)
startAttempt := int32(1)
if options.Attempt > 0 {
	startAttempt = int32(options.Attempt)
}
var parentJobID *pgtype.UUID
if options.ParentJobID.Valid {
	parentJobID = &options.ParentJobID
}
var lastErr error
for attempt := startAttempt; attempt <= maxAttempts; attempt++ {
	result, err := provider.Run(ctx, intent)
	if err == nil {
		return s.persistSuccessfulRun(ctx, node, intent, result, parentJobID, attempt, maxAttempts)
	}
	failedJob, jobErr := s.createFailedJob(ctx, node, intent, err, parentJobID, attempt, maxAttempts)
	if jobErr != nil {
		return RunResult{}, jobErr
	}
	parentJobID = &failedJob.ID
	lastErr = err
}
return RunResult{}, fmt.Errorf("%w: exhausted %d attempts after %v", ErrRetryExhausted, maxAttempts, lastErr)
```

Add error:

```go
var ErrRetryExhausted = errors.New("retry exhausted")
```

Update `errorCodeForRun`:

```go
case errors.Is(err, ErrRetryExhausted):
	return "retry_exhausted"
```

In `apps/server/internal/api/run_handler.go`, update `statusForRunError` so `ErrRetryExhausted` returns `400`.

Extract the existing success transaction into:

```go
func (s *Service) persistSuccessfulRun(
	ctx context.Context,
	node db.MediaNode,
	intent GenerationIntent,
	result ProviderResult,
	parentJobID *pgtype.UUID,
	attempt int32,
	maxAttempts int32,
) (RunResult, error)
```

The success job params must set `ParentJobID`, `Attempt`, and `MaxAttempts`.

- [ ] **Step 3: Keep non-text success unsupported after validation**

At the beginning of `persistSuccessfulRun`, add:

```go
if node.NodeType != db.NodeTypeText {
	err := fmt.Errorf("%w: successful %s output persistence is not implemented", ErrProviderUnavailable, node.NodeType)
	failedJob, jobErr := s.createFailedJob(ctx, node, intent, err, parentJobID, attempt, maxAttempts)
	if jobErr != nil {
		return RunResult{}, jobErr
	}
	return RunResult{Job: failedJob}, err
}
```

This preserves M4.3's validation behavior without pretending image/video success persistence exists.

- [ ] **Step 4: Run production tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -count=1
```

Expected: PASS.

---

### Task 7: Add Run Options And Manual Retry API

**Files:**
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/production/service.go`

- [ ] **Step 1: Add run option parsing tests**

Add these tests to `apps/server/internal/api/run_handler_test.go`:

```go
func TestRunNodeRequestDefaultsMaxAttempts(t *testing.T) {
	req := runNodeRequest{}
	if got := req.runOptions().MaxAttempts; got != 1 {
		t.Fatalf("max attempts = %d, want 1", got)
	}
}

func TestRunNodeRequestCapsMaxAttempts(t *testing.T) {
	req := runNodeRequest{MaxAttempts: 99}
	if got := req.runOptions().MaxAttempts; got != 3 {
		t.Fatalf("max attempts = %d, want 3", got)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestRunNodeRequest' -count=1
```

Expected: FAIL because `runNodeRequest` does not exist.

- [ ] **Step 3: Add run request DTO**

In `apps/server/internal/api/run_handler.go`, add:

```go
type runNodeRequest struct {
	MaxAttempts int `json:"max_attempts"`
}

func (r runNodeRequest) runOptions() production.RunOptions {
	maxAttempts := r.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if maxAttempts > 3 {
		maxAttempts = 3
	}
	return production.RunOptions{MaxAttempts: maxAttempts}
}
```

In `RunNode`, bind the body without requiring one:

```go
var req runNodeRequest
if len(c.Request.Body()) > 0 {
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
}
```

Pass `req.runOptions()` to `RunNode`.

- [ ] **Step 4: Add retry service method**

In `apps/server/internal/production/service.go`, add:

```go
func (s *Service) RetryJob(ctx context.Context, jobID pgtype.UUID, requestedBy RequestedBy) (RunResult, error) {
	job, err := s.queries.GetGenerationJobByID(ctx, jobID)
	if err != nil {
		return RunResult{}, err
	}
	if job.Status != db.JobStatusFailed {
		return RunResult{}, fmt.Errorf("%w: only failed jobs can be retried", ErrCapabilityMismatch)
	}
	latest, err := s.queries.LatestGenerationJobInChain(ctx, job.ID)
	if err != nil {
		return RunResult{}, err
	}
	if latest.Attempt >= latest.MaxAttempts {
		return RunResult{}, ErrRetryExhausted
	}
	return s.runNodeAttempt(ctx, latest.TargetNodeID, requestedBy, RunOptions{
		MaxAttempts: int(latest.MaxAttempts),
		ParentJobID: latest.ID,
		Attempt: int(latest.Attempt + 1),
	})
}
```

If `runNodeAttempt` does not exist yet, extract the body of `RunNode` into:

```go
func (s *Service) runNodeAttempt(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy, options RunOptions) (RunResult, error)
```

Then have `RunNode` call it with `RunOptions`.

- [ ] **Step 5: Expose attempt fields in job responses**

In `apps/server/internal/api/run_handler.go`, extend `generationJobResponse`:

```go
type generationJobResponse struct {
	ID               string         `json:"id"`
	TargetNodeID     string         `json:"target_node_id"`
	ParentJobID      string         `json:"parent_job_id,omitempty"`
	OperationType    string         `json:"operation_type"`
	Provider         string         `json:"provider"`
	ModelID          string         `json:"model_id"`
	Intent           map[string]any `json:"intent"`
	RenderedPrompt   string         `json:"rendered_prompt"`
	ProviderRequest  map[string]any `json:"provider_request"`
	ProviderResponse map[string]any `json:"provider_response"`
	Status           string         `json:"status"`
	Attempt          int32          `json:"attempt"`
	MaxAttempts      int32          `json:"max_attempts"`
	ErrorCode        string         `json:"error_code,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	RequestedByType  string         `json:"requested_by_type"`
	RequestedByID    string         `json:"requested_by_id,omitempty"`
}
```

Update `toGenerationJobResponse`:

```go
ParentJobID:      uuidString(job.ParentJobID),
Attempt:          job.Attempt,
MaxAttempts:      job.MaxAttempts,
```

Add helper:

```go
func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuidToString(value)
}
```

- [ ] **Step 6: Add retry handler**

In `apps/server/internal/api/run_handler.go`, add:

```go
func (h *RunHandler) RetryJob(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	jobID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "job not found")
		return
	}
	job, err := h.queries.GetGenerationJobByID(ctx, jobID)
	if err != nil {
		writeError(c, consts.StatusNotFound, "job not found")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, uuidToString(job.TargetNodeID), accountID, c)
	if !ok {
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
		return
	}
	result, err := h.service.RetryJob(ctx, jobID, production.RequestedBy{Type: "user", ID: uuidToString(accountID)})
	if err != nil {
		latest, latestErr := h.queries.LatestGenerationJobInChain(ctx, jobID)
		if latestErr == nil {
			c.JSON(statusForRunError(err), runNodeResponse{Job: toGenerationJobResponse(latest)})
			return
		}
		writeError(c, statusForRunError(err), err.Error())
		return
	}
	c.JSON(consts.StatusOK, runNodeResponse{
		Node:    result.Node,
		Job:     toGenerationJobResponse(result.Job),
		Version: result.Version,
	})
}
```

Wire route in `apps/server/cmd/server/main.go`:

```go
h.POST("/api/jobs/:id/retry", authMiddleware, runHandler.RetryJob)
```

- [ ] **Step 6: Run API tests and server build**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestRunNodeRequest|TestRunJobResponse|TestNewRunHandler' -count=1
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

---

### Task 8: Add M4.3 Smoke Script

**Files:**
- Create: `scripts/smoke-m4-3.sh`

- [ ] **Step 1: Create smoke script**

Create executable `scripts/smoke-m4-3.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

node <<'NODE'
const base = process.env.CLIPANVIL_API_BASE || `http://127.0.0.1:${process.env.CLIPANVIL_SERVER_PORT || "8888"}/api`;

async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

async function reqAllowError(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  return {status: res.status, body: text ? JSON.parse(text) : null};
}

const email = `m4-3-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.3 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({name: "M4.3 Smoke", mode: "studio"}),
});

const mismatchNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "video",
    title: "Capability mismatch",
    prompt: "make a video",
    operation_type: "text_to_video",
    model_provider: "mock",
    model_id: "mock-image-only",
    model_params: {},
    canvas_x: 20,
    canvas_y: 40,
  }),
});
const mismatch = await reqAllowError(`/nodes/${mismatchNode.id}/run`, {method: "POST", headers});
if (mismatch.status !== 400 || mismatch.body.job?.error_code !== "capability_mismatch") {
  throw new Error(`capability mismatch was not persisted: ${JSON.stringify(mismatch)}`);
}
if (mismatch.body.job.provider_response.code !== "capability_mismatch") {
  throw new Error(`capability response missing code: ${JSON.stringify(mismatch.body.job)}`);
}

const failNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Mock failure",
    prompt: "force provider failure",
    operation_type: "text_generation",
    model_provider: "mock",
    model_id: "mock-text",
    model_params: {mock_fail: true},
    canvas_x: 40,
    canvas_y: 80,
  }),
});
const failedRun = await reqAllowError(`/nodes/${failNode.id}/run`, {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({max_attempts: 2}),
});
if (failedRun.status !== 500 || failedRun.body.job?.status !== "failed") {
  throw new Error(`provider failure did not return failed job: ${JSON.stringify(failedRun)}`);
}
const jobsAfterAuto = await req(`/nodes/${failNode.id}/jobs`, {headers});
if (jobsAfterAuto.length !== 2) {
  throw new Error(`auto retry should create exactly 2 jobs, got ${jobsAfterAuto.length}`);
}
if (jobsAfterAuto[1].attempt !== 2 && jobsAfterAuto[1].intent) {
  throw new Error(`second job is not attempt 2: ${JSON.stringify(jobsAfterAuto[1])}`);
}
if (!jobsAfterAuto[1].id || jobsAfterAuto[1].id === jobsAfterAuto[0].id) {
  throw new Error(`retry did not create a distinct job: ${JSON.stringify(jobsAfterAuto)}`);
}

const retryAfterMax = await reqAllowError(`/jobs/${jobsAfterAuto[1].id}/retry`, {
  method: "POST",
  headers,
});
if (retryAfterMax.status !== 400) {
  throw new Error(`retry after max returned unexpected status ${retryAfterMax.status}`);
}
const jobsAfterExhaust = await req(`/nodes/${failNode.id}/jobs`, {headers});
if (jobsAfterExhaust.length !== 2) {
  throw new Error(`retry beyond max created extra job: ${jobsAfterExhaust.length}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  mismatchNodeId: mismatchNode.id,
  mismatchJobId: mismatch.body.job.id,
  failedNodeId: failNode.id,
  failedAttempts: jobsAfterExhaust.length,
  latestFailedJobId: jobsAfterExhaust[jobsAfterExhaust.length - 1].id,
}, null, 2));
NODE
```

- [ ] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/smoke-m4-3.sh
```

Expected: executable bit is set.

---

### Task 9: Full Verification And Milestone Record

**Files:**
- Modify: `docs/milestones/m4-shared-production-foundation.md`

- [ ] **Step 1: Run required verification**

Run:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all pass.

- [ ] **Step 2: Run M4.1 and M4.2 regression smoke**

Start the backend for the current worktree, then run:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-1.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-2.sh
```

Expected: both pass.

- [ ] **Step 3: Run M4.3 smoke**

Run:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-3.sh
```

Expected: PASS and JSON output containing a capability mismatch job, a provider failure job chain, and no job beyond max attempts.

- [ ] **Step 4: Record completion**

Under `### M4.3 Capability Validation, Failure Records, And Retry` in `docs/milestones/m4-shared-production-foundation.md`, append:

```markdown
Completion record:

- Added `model_provider` and `model_capability` tables with mock and internal capability seeds.
- Node runs validate output type, operation, prompt limits, and selected model capability before provider execution.
- Capability mismatches persist failed jobs with `error_code=capability_mismatch`.
- Mock provider failures persist failed jobs with provider response summaries.
- Automatic retries create bounded `generation_job` chains using `parent_job_id`, `attempt`, and `max_attempts`.
- Retry requests do not create attempts beyond the configured maximum.

Verification:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-1.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-2.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-3.sh
git diff --check
```
```

---

## E2E Acceptance Gate

M4.3 is complete only when this gate passes:

1. `model_provider` and `model_capability` exist and include mock seed capabilities.
2. A node selecting an incompatible model fails before provider execution.
3. The incompatible run persists a failed `generation_job` with `error_code=capability_mismatch`, readable `error_message`, and `attempt=1`.
4. A supported text node with `model_params.mock_fail=true` persists failed provider jobs.
5. A retry creates a new job with `parent_job_id` pointing to the previous failed job and incremented `attempt`.
6. Automatic retry stops at `max_attempts` and does not create extra jobs beyond the cap.
7. M4.1 and M4.2 smoke tests still pass.

## Execution Note

The user has been executing M4 inline. Implement this plan directly in the current session with `superpowers:executing-plans`, task by task, and run the full acceptance gate before marking M4.3 complete.
