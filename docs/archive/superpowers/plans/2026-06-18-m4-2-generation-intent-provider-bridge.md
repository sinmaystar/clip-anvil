# M4.2 GenerationIntent And Provider Bridge Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Do not use subagents for this milestone unless the user explicitly changes that instruction.

**Goal:** Move node execution behind a stable `GenerationIntent` and provider registry boundary so Studio runs and future Agent Worker runs share one production contract.

**Architecture:** Expand the M4.1 text-only production service into three layers: intent building, provider registry/bridge, and job persistence. Mock providers remain the default for tests and local smoke; Volcengine is added as a configured adapter boundary that fails before network calls when required credentials are absent.

**Tech Stack:** Go 1.26, Hertz, pgx v5, sqlc v1.31, Viper config/env overrides, PostgreSQL `generation_job` persistence, Node-based smoke scripts.

---

## Spec Source

- Milestone: `docs/milestones/m4-shared-production-foundation.md`
- Parent roadmap: `docs/milestones/m3-m6-studio-agent-roadmap.md`
- Shared production design: `docs/superpowers/specs/2026-06-18-studio-agent-shared-production-design.md`
- Database design: `docs/superpowers/specs/2026-06-18-production-database-technical-design.md`
- M4.1 baseline plan: `docs/superpowers/plans/2026-06-18-m4-1-schema-mock-text-run.md`

M4.2 acceptance to satisfy:

- Node run requests are converted to a `GenerationIntent` before provider execution.
- Mock providers are selected by capability/config.
- Tests do not need external API keys.
- Missing API keys fail clearly when a real provider is requested.
- `generation_job.intent`, `rendered_prompt`, `provider_request`, `provider_response`, and requested-by fields are persisted.

Non-goals for M4.2:

- Full `model_provider` / `model_capability` tables. That belongs to M4.3.
- Real Volcengine API calls. M4.2 only adds the adapter boundary and credential/config failure path.
- Reference Pack expansion, stale propagation, retry chains, and media extraction. Those belong to M4.4/M4.5.

## File Structure

- Modify `apps/server/internal/config/config.go`: add production/provider config and bind env keys.
- Modify `apps/server/internal/config/config_test.go`: cover env overrides and local `.env` loading.
- Modify `apps/server/config.yaml`: add committed mock-first provider defaults without secrets.
- Create `.env.example`: document optional local provider credentials and model choices.
- Modify `apps/server/internal/production/intent.go`: replace the M4.1 flat intent with a stable nested contract.
- Create `apps/server/internal/production/provider.go`: provider interface, registry, config errors, selection rules.
- Modify `apps/server/internal/production/mock_provider.go`: implement the registry provider interface.
- Create `apps/server/internal/production/volcengine_provider.go`: env-backed stub adapter that fails before network calls when API key is missing.
- Modify `apps/server/internal/production/service.go`: build intent first, create failed jobs for bridge/config errors, then persist success.
- Modify `apps/server/internal/production/service_test.go`: unit-test intent node, registry selection, mock run, and missing API key error.
- Modify `apps/server/sqlc/queries/production.sql`: add job lookup queries needed by API/smoke.
- Regenerate `apps/server/internal/store/db/production.sql.go`.
- Modify `apps/server/internal/api/node_handler.go`: accept production fields on create/update.
- Modify `apps/server/internal/api/node_handler_test.go`: cover production field request parsing/validation helpers.
- Modify `apps/server/internal/api/run_handler.go`: return run job summary and expose job listing.
- Modify `apps/server/internal/api/run_handler_test.go`: cover response DTO helpers.
- Modify `apps/server/cmd/server/main.go`: create provider registry from config and wire job route.
- Create `scripts/smoke-m4-2.sh`: E2E smoke for mock intent persistence and real-provider missing-key failure.
- Modify `docs/milestones/m4-shared-production-foundation.md`: add M4.2 completion record after implementation.

---

### Task 1: Add Production Provider Config

**Files:**
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Modify: `apps/server/config.yaml`
- Create: `.env.example`

- [ ] **Step 1: Write failing config tests**

Add these tests to `apps/server/internal/config/config_test.go`:

```go
func TestLoadProductionConfigDefaultsFromYaml(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
server:
  port: 9999
postgres:
  dsn: "postgres://test:test@localhost:5432/test?sslmode=disable"
redis:
  addr: "localhost:6379"
minio:
  endpoint: "localhost:9000"
  sandbox_endpoint: "host.docker.internal:9000"
  access_key: "clipanvil"
  secret_key: "clipanvil_dev"
  use_ssl: false
jwt:
  secret: "test-secret"
  expire_hours: 12
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
  volcengine:
    base_url: "https://ark.cn-beijing.volces.com"
    text_model: "doubao-seed-1-6-lite"
    image_model: "seedream-lite"
    video_model: "seedance-lite"
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Production.ProviderMode != "mock" {
		t.Fatalf("ProviderMode = %q, want mock", cfg.Production.ProviderMode)
	}
	if cfg.Production.DefaultTextModel != "mock-text" {
		t.Fatalf("DefaultTextModel = %q, want mock-text", cfg.Production.DefaultTextModel)
	}
	if cfg.Production.Volcengine.APIKey != "" {
		t.Fatalf("Volcengine.APIKey must not be set by committed yaml")
	}
}

func TestLoadProductionConfigAllowsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
server:
  port: 9999
postgres:
  dsn: "postgres://test:test@localhost:5432/test?sslmode=disable"
redis:
  addr: "localhost:6379"
minio:
  endpoint: "localhost:9000"
  sandbox_endpoint: "host.docker.internal:9000"
  access_key: "clipanvil"
  secret_key: "clipanvil_dev"
  use_ssl: false
jwt:
  secret: "test-secret"
  expire_hours: 12
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
  volcengine:
    base_url: "https://example.invalid"
    text_model: "cheap-default"
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)
	t.Setenv("CLIPANVIL_PRODUCTION_PROVIDER_MODE", "real")
	t.Setenv("CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER", "volcengine")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY", "local-key")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL", "doubao-cheap")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Production.ProviderMode != "real" {
		t.Fatalf("ProviderMode = %q, want real", cfg.Production.ProviderMode)
	}
	if cfg.Production.DefaultProvider != "volcengine" {
		t.Fatalf("DefaultProvider = %q, want volcengine", cfg.Production.DefaultProvider)
	}
	if cfg.Production.Volcengine.APIKey != "local-key" {
		t.Fatalf("Volcengine.APIKey was not loaded from env")
	}
	if cfg.Production.Volcengine.TextModel != "doubao-cheap" {
		t.Fatalf("Volcengine.TextModel = %q, want doubao-cheap", cfg.Production.Volcengine.TextModel)
	}
}

func TestLoadProductionConfigReadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
server:
  port: 9999
postgres:
  dsn: "postgres://test:test@localhost:5432/test?sslmode=disable"
redis:
  addr: "localhost:6379"
minio:
  endpoint: "localhost:9000"
  sandbox_endpoint: "host.docker.internal:9000"
  access_key: "clipanvil"
  secret_key: "clipanvil_dev"
  use_ssl: false
jwt:
  secret: "test-secret"
  expire_hours: 12
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	envData := []byte(`
CLIPANVIL_PRODUCTION_PROVIDER_MODE=real
CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER=volcengine
CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY=dotenv-key
`)
	if err := os.WriteFile(filepath.Join(dir, ".env"), envData, 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Production.ProviderMode != "real" {
		t.Fatalf("ProviderMode = %q, want real", cfg.Production.ProviderMode)
	}
	if cfg.Production.DefaultProvider != "volcengine" {
		t.Fatalf("DefaultProvider = %q, want volcengine", cfg.Production.DefaultProvider)
	}
	if cfg.Production.Volcengine.APIKey != "dotenv-key" {
		t.Fatalf("Volcengine.APIKey was not loaded from .env")
	}
}
```

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config -run 'TestLoadProductionConfig' -count=1
```

Expected: FAIL because `Config.Production` and env edges do not exist yet.

- [ ] **Step 3: Implement production config**

Update `apps/server/internal/config/config.go` with these structs and bind keys:

```go
type Config struct {
	Server     ServerConfig
	Postgres   PostgresConfig
	Redis      RedisConfig
	MinIO      MinIOConfig
	JWT        JWTConfig
	Sandbox    SandboxConfig
	Production ProductionConfig
}

type ProductionConfig struct {
	ProviderMode     string `mapstructure:"provider_mode"`
	DefaultProvider  string `mapstructure:"default_provider"`
	DefaultTextModel string `mapstructure:"default_text_model"`
	Volcengine       VolcengineConfig
}

type VolcengineConfig struct {
	APIKey     string `mapstructure:"api_key"`
	BaseURL    string `mapstructure:"base_url"`
	TextModel  string `mapstructure:"text_model"`
	ImageModel string `mapstructure:"image_model"`
	VideoModel string `mapstructure:"video_model"`
}
```

Append these keys in `bindEnv`:

```go
"production.provider_mode",
"production.default_provider",
"production.default_text_model",
"production.volcengine.api_key",
"production.volcengine.base_url",
"production.volcengine.text_model",
"production.volcengine.image_model",
"production.volcengine.video_model",
```

- [ ] **Step 4: Load local `.env` before Viper reads env overrides**

At the start of `Load`, call the local env parser before `v.AutomaticEnv()`:

```go
func Load() (*Config, error) {
	_ = loadDotEnv(".env")

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./apps/server")
	v.AddConfigPath("../..")
	v.SetEnvPrefix("CLIPANVIL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := bindEnv(v); err != nil {
		return nil, err
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

Add this parser in the same file:

```go
func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
```

Update `config.go` imports:

```go
import (
	"os"
	"strings"

	"github.com/spf13/viper"
)
```

- [ ] **Step 5: Add committed mock-first defaults**

Append this block to `apps/server/config.yaml`:

```yaml
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
  volcengine:
    base_url: "https://ark.cn-beijing.volces.com"
    text_model: "doubao-seed-1-6-lite"
    image_model: "seedream-lite"
    video_model: "seedance-lite"
```

- [ ] **Step 6: Document local `.env` keys without committing secrets**

Create `.env.example`:

```dotenv
# Optional local provider settings. Copy to .env for private values.
# Automated tests and default local smoke use mock providers and do not need these.

CLIPANVIL_PRODUCTION_PROVIDER_MODE=mock
CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER=mock
CLIPANVIL_PRODUCTION_DEFAULT_TEXT_MODEL=mock-text

# Set these only when manually testing Volcengine adapter work.
CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY=
CLIPANVIL_PRODUCTION_VOLCENGINE_BASE_URL=https://ark.cn-beijing.volces.com
CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL=doubao-seed-1-6-lite
CLIPANVIL_PRODUCTION_VOLCENGINE_IMAGE_MODEL=seedream-lite
CLIPANVIL_PRODUCTION_VOLCENGINE_VIDEO_MODEL=seedance-lite
```

- [ ] **Step 7: Run config tests and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config -run 'TestLoadProductionConfig' -count=1
```

Expected: PASS.

---

### Task 2: Define Stable GenerationIntent Contract

**Files:**
- Modify: `apps/server/internal/production/intent.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Write failing intent-node tests**

Add these tests to `apps/server/internal/production/service_test.go`:

```go
func TestGenerationIntentJSONNode(t *testing.T) {
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
```

Add imports:

```go
import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)
```

- [ ] **Step 2: Run intent test and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run TestGenerationIntentJSONNode -count=1
```

Expected: FAIL because the current `GenerationIntent` still uses flat `model_provider/model_id` fields.

- [ ] **Step 3: Replace intent structs**

Replace `apps/server/internal/production/intent.go` with:

```go
package production

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type GenerationIntent struct {
	WorkspaceID    pgtype.UUID    `json:"workspace_id"`
	TargetNodeID   pgtype.UUID    `json:"target_node_id"`
	OutputType     string         `json:"output_type"`
	OperationType  string         `json:"operation_type"`
	PromptTemplate string         `json:"prompt_template"`
	InputRefs      []InputRef     `json:"input_refs"`
	Model          ModelSpec      `json:"model"`
	Params         map[string]any `json:"params"`
	RequestedBy    RequestedBy    `json:"requested_by"`
}

type InputRef struct {
	NodeID   pgtype.UUID `json:"node_id"`
	Kind     string      `json:"kind"`
	Required bool        `json:"required"`
}

type ModelSpec struct {
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
}

type RequestedBy struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

type ProviderResult struct {
	RenderedPrompt   string
	TextContent      string
	ProviderRequest  map[string]any
	ProviderResponse map[string]any
}

type ProviderBridge interface {
	Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error)
}
```

- [ ] **Step 4: Run intent test and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run TestGenerationIntentJSONNode -count=1
```

Expected: PASS after dependent mock provider compile fixes from Task 3 are in place. If compile fails now, continue to Task 3 and rerun.

---

### Task 3: Add Provider Registry And Mock Provider Bridge

**Files:**
- Create: `apps/server/internal/production/provider.go`
- Modify: `apps/server/internal/production/mock_provider.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Write failing provider registry tests**

Add these tests to `apps/server/internal/production/service_test.go`:

```go
func TestProviderRegistrySelectsMockProvider(t *testing.T) {
	registry := NewProviderRegistry(ProviderConfig{
		ProviderMode:     "mock",
		DefaultProvider:  "mock",
		DefaultTextModel: "mock-text",
	})

	intent := GenerationIntent{
		OutputType:     "text",
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "", ModelID: ""},
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
```

Add `errors` to imports.

- [ ] **Step 2: Run provider tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestProviderRegistry' -count=1
```

Expected: FAIL because registry types do not exist.

- [ ] **Step 3: Implement provider registry**

Create `apps/server/internal/production/provider.go`:

```go
package production

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrProviderConfig      = errors.New("provider configuration error")
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
			"mock":       MockProvider{},
			"volcengine": NewVolcengineProvider(cfg.Volcengine),
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
```

- [ ] **Step 4: Convert mock provider to the bridge interface**

Replace `apps/server/internal/production/mock_provider.go` with:

```go
package production

import (
	"context"
	"fmt"
)

type MockProvider struct{}

func (MockProvider) Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	select {
	case <-ctx.Done():
		return ProviderResult{}, ctx.Err()
	default:
	}

	rendered := intent.PromptTemplate
	if rendered == "" {
		rendered = "empty prompt"
	}
	text := fmt.Sprintf("[mock:%s] %s", intent.Model.ModelID, rendered)
	return ProviderResult{
		RenderedPrompt: rendered,
		TextContent:    text,
		ProviderRequest: map[string]any{
			"provider":       intent.Model.Provider,
			"model_id":       intent.Model.ModelID,
			"operation_type": intent.OperationType,
			"prompt":         rendered,
			"params":         intent.Params,
		},
		ProviderResponse: map[string]any{
			"provider": "mock",
			"text":     text,
		},
	}, nil
}
```

- [ ] **Step 5: Update existing mock provider test**

Replace `TestMockTextProviderReturnsDeterministicText` with:

```go
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
```

- [ ] **Step 6: Run provider tests and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestProviderRegistry|TestMockProvider' -count=1
```

Expected: PASS after Task 4 creates `NewVolcengineProvider`.

---

### Task 4: Add Volcengine Adapter Boundary

**Files:**
- Create: `apps/server/internal/production/volcengine_provider.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Write failing missing-key test**

Add this test:

```go
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
```

- [ ] **Step 2: Run missing-key test and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run TestVolcengineProviderFailsBeforeNetworkWithoutAPIKey -count=1
```

Expected: FAIL because `NewVolcengineProvider` does not exist.

- [ ] **Step 3: Implement Volcengine stub adapter**

Create `apps/server/internal/production/volcengine_provider.go`:

```go
package production

import (
	"context"
	"fmt"
	"strings"
)

type VolcengineProvider struct {
	cfg VolcengineProviderConfig
}

func NewVolcengineProvider(cfg VolcengineProviderConfig) VolcengineProvider {
	return VolcengineProvider{cfg: cfg}
}

func (p VolcengineProvider) Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	select {
	case <-ctx.Done():
		return ProviderResult{}, ctx.Err()
	default:
	}

	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return ProviderResult{}, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for provider volcengine", ErrProviderConfig)
	}

	return ProviderResult{}, fmt.Errorf("%w: volcengine %s adapter is not implemented in M4.2", ErrProviderUnavailable, intent.OutputType)
}
```

- [ ] **Step 4: Run production unit tests and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -count=1
```

Expected: PASS.

---

### Task 5: Persist Failed Jobs For Bridge/Config Errors

**Files:**
- Modify: `apps/server/internal/production/service.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Add service helper tests**

Add lightweight helper tests that do not need a database:

```go
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
```

Add imports:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)
```

- [ ] **Step 2: Run helper tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestErrorCodeForProviderConfig|TestIntentForNodeUsesProductionFields' -count=1
```

Expected: FAIL because `errorCodeForRun` and the new `intentForNode` signature do not exist.

- [ ] **Step 3: Refactor service fields and constructor**

In `apps/server/internal/production/service.go`, change the service struct and constructor to:

```go
type Service struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	registry *ProviderRegistry
}

func NewService(pool *pgxpool.Pool, queries *db.Queries, registry *ProviderRegistry) *Service {
	return &Service{pool: pool, queries: queries, registry: registry}
}
```

- [ ] **Step 4: Build intent before provider execution**

Replace the start of `RunNode` with:

```go
func (s *Service) RunNode(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy) (RunResult, error) {
	node, err := s.queries.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		return RunResult{}, err
	}
	if node.NodeType != db.NodeTypeText {
		intent := intentForNode(node, requestedBy)
		if jobErr := s.createFailedJob(ctx, node, intent, ErrUnsupportedNodeType); jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{}, ErrUnsupportedNodeType
	}

	intent := s.registry.ApplyDefaults(intentForNode(node, requestedBy))
	provider, err := s.registry.Resolve(intent)
	if err != nil {
		if jobErr := s.createFailedJob(ctx, node, intent, err); jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{}, err
	}

	result, err := provider.Run(ctx, intent)
	if err != nil {
		if jobErr := s.createFailedJob(ctx, node, intent, err); jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{}, err
	}
```

Define the return type near the service struct:

```go
type RunResult struct {
	Node    db.MediaNode
	Job     db.GenerationJob
	Version db.ArtifactVersion
}
```

- [ ] **Step 5: Return job and version from successful runs**

At the end of the successful transaction, replace `return updated, nil` with:

```go
return RunResult{Node: updated, Job: job, Version: version}, nil
```

- [ ] **Step 6: Add failed job helper**

Add these helpers to `service.go`:

```go
func (s *Service) createFailedJob(ctx context.Context, node db.MediaNode, intent GenerationIntent, runErr error) error {
	intentJSON, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = s.queries.CreateGenerationJob(ctx, db.CreateGenerationJobParams{
		WorkspaceID:       node.WorkspaceID,
		TargetNodeID:      node.ID,
		OperationType:     intent.OperationType,
		Provider:          intent.Model.Provider,
		ModelID:           intent.Model.ModelID,
		Intent:            intentJSON,
		RenderedPrompt:    intent.PromptTemplate,
		ProviderRequest:   []byte("{}"),
		ProviderResponse:  []byte("{}"),
		Status:            db.JobStatusFailed,
		Progress:          0,
		Attempt:           1,
		MaxAttempts:       1,
		RetryPolicy:       []byte("{}"),
		ErrorCode:         pgtype.Text{String: errorCodeForRun(runErr), Valid: true},
		ErrorMessage:      pgtype.Text{String: runErr.Error(), Valid: true},
		RequestedByType:   intent.RequestedBy.Type,
		RequestedByID:     nullableText(intent.RequestedBy.ID),
		StartedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		CompletedAt:       pgtype.Timestamptz{Time: now, Valid: true},
	})
	return err
}

func errorCodeForRun(err error) string {
	switch {
	case errors.Is(err, ErrUnsupportedNodeType):
		return "unsupported_node_type"
	case errors.Is(err, ErrProviderConfig):
		return "provider_config_error"
	case errors.Is(err, ErrProviderUnavailable):
		return "provider_unavailable"
	default:
		return "provider_error"
	}
}
```

- [ ] **Step 7: Update intent builder**

Replace `intentForNode` with:

```go
func intentForNode(node db.MediaNode, requestedBy RequestedBy) GenerationIntent {
	prompt := node.PromptTemplate
	if prompt == "" {
		prompt = node.Prompt
	}
	operation := node.OperationType
	if operation == "" || operation == "manual" {
		operation = "text_generation"
	}
	provider := ""
	if node.ModelProvider.Valid {
		provider = node.ModelProvider.String
	}
	modelID := ""
	if node.ModelID.Valid {
		modelID = node.ModelID.String
	}
	params := map[string]any{}
	if len(node.ModelParams) > 0 {
		_ = json.Unmarshal(node.ModelParams, &params)
	}
	return GenerationIntent{
		WorkspaceID:    node.WorkspaceID,
		TargetNodeID:   node.ID,
		OutputType:     string(node.NodeType),
		OperationType:  operation,
		PromptTemplate: prompt,
		InputRefs:      []InputRef{},
		Model:          ModelSpec{Provider: provider, ModelID: modelID},
		Params:         params,
		RequestedBy:    requestedBy,
	}
}
```

- [ ] **Step 8: Run production tests and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -count=1
```

Expected: PASS.

---

### Task 6: Expose Production Node Config In Studio API

**Files:**
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/node_handler_test.go`

- [ ] **Step 1: Write request helper tests**

Add tests in `apps/server/internal/api/node_handler_test.go`:

```go
func TestCreateNodeRequestModelParamsJSON(t *testing.T) {
	raw := json.RawMessage(`{"temperature":0.2}`)
	req := createNodeRequest{ModelParams: raw}
	got := req.modelParamsJSON()
	if string(got) != `{"temperature":0.2}` {
		t.Fatalf("model params = %s", got)
	}
}

func TestCreateNodeRequestModelParamsDefaultsToObject(t *testing.T) {
	req := createNodeRequest{}
	got := req.modelParamsJSON()
	if string(got) != `{}` {
		t.Fatalf("model params = %s", got)
	}
}
```

Add `encoding/json` import if needed.

- [ ] **Step 2: Run node handler tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestCreateNodeRequestModelParams' -count=1
```

Expected: FAIL because `ModelParams` and helper do not exist.

- [ ] **Step 3: Extend create/update request DTOs**

In `createNodeRequest`, add:

```go
OperationType string          `json:"operation_type"`
ModelProvider string          `json:"model_provider"`
ModelID       string          `json:"model_id"`
ModelParams   json.RawMessage `json:"model_params"`
```

In `updateNodeRequest`, add:

```go
OperationType *string          `json:"operation_type"`
ModelProvider *string          `json:"model_provider"`
ModelID       *string          `json:"model_id"`
ModelParams   *json.RawMessage `json:"model_params"`
```

Update `hasChanges`:

```go
return r.Title != nil || r.Prompt != nil || r.Status != nil || r.GroupID != nil ||
	r.OperationType != nil || r.ModelProvider != nil || r.ModelID != nil || r.ModelParams != nil
```

Add import:

```go
import "encoding/json"
```

- [ ] **Step 4: Add request helper methods**

Add near `createNodeRequest` methods:

```go
func (r createNodeRequest) modelParamsJSON() []byte {
	if len(r.ModelParams) == 0 {
		return []byte("{}")
	}
	return []byte(r.ModelParams)
}

func nullableString(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}
```

- [ ] **Step 5: Apply production config after node create**

After `CreateMediaNode` / `CreateMediaNodeWithID` succeeds, add:

```go
if req.OperationType != "" || req.ModelProvider != "" || req.ModelID != "" || len(req.ModelParams) > 0 {
	operation := strings.TrimSpace(req.OperationType)
	if operation == "" {
		operation = node.OperationType
	}
	node, err = h.queries.UpdateMediaNodeProductionConfig(ctx, db.UpdateMediaNodeProductionConfigParams{
		ID:             node.ID,
		OperationType:  operation,
		PromptTemplate: node.PromptTemplate,
		ModelProvider:  nullableString(req.ModelProvider),
		ModelID:        nullableString(req.ModelID),
		ModelParams:    req.modelParamsJSON(),
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create node")
		return
	}
}
```

- [ ] **Step 6: Apply production config on update**

After prompt/title/status/group updates, add:

```go
if req.OperationType != nil || req.ModelProvider != nil || req.ModelID != nil || req.ModelParams != nil {
	operation := node.OperationType
	if req.OperationType != nil {
		operation = strings.TrimSpace(*req.OperationType)
	}
	prompt := node.PromptTemplate
	provider := node.ModelProvider
	if req.ModelProvider != nil {
		provider = nullableString(*req.ModelProvider)
	}
	modelID := node.ModelID
	if req.ModelID != nil {
		modelID = nullableString(*req.ModelID)
	}
	modelParams := node.ModelParams
	if req.ModelParams != nil {
		modelParams = []byte(*req.ModelParams)
		if len(modelParams) == 0 {
			modelParams = []byte("{}")
		}
	}
	node, err = h.queries.UpdateMediaNodeProductionConfig(ctx, db.UpdateMediaNodeProductionConfigParams{
		ID:             node.ID,
		OperationType:  operation,
		PromptTemplate: prompt,
		ModelProvider:  provider,
		ModelID:        modelID,
		ModelParams:    modelParams,
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update node")
		return
	}
}
```

- [ ] **Step 7: Run API tests and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestCreateNodeRequestModelParams' -count=1
```

Expected: PASS.

---

### Task 7: Add Job Query API And Run Response

**Files:**
- Modify: `apps/server/sqlc/queries/production.sql`
- Regenerate: `apps/server/internal/store/db/production.sql.go`
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Add sqlc job queries**

Append to `apps/server/sqlc/queries/production.sql`:

```sql
-- name: GetGenerationJobByID :one
SELECT *
FROM generation_job
WHERE id = $1;

-- name: LatestGenerationJobByNode :one
SELECT *
FROM generation_job
WHERE target_node_id = $1
ORDER BY created_at DESC
LIMIT 1;
```

- [ ] **Step 2: Regenerate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: `apps/server/internal/store/db/production.sql.go` is updated with the new queries.

- [ ] **Step 3: Add response DTO tests**

In `apps/server/internal/api/run_handler_test.go`, add:

```go
func TestRunJobResponseExposesIntentAndSummaries(t *testing.T) {
	job := db.GenerationJob{
		OperationType:     "text_generation",
		Provider:          "mock",
		ModelID:           "mock-text",
		Intent:            []byte(`{"model":{"provider":"mock","model_id":"mock-text"}}`),
		RenderedPrompt:    "write a short ad",
		ProviderRequest:   []byte(`{"provider":"mock"}`),
		ProviderResponse:  []byte(`{"text":"ok"}`),
		Status:            db.JobStatusSucceeded,
		RequestedByType:   "user",
		RequestedByID:     pgtype.Text{String: "account-123", Valid: true},
	}

	resp := toGenerationJobResponse(job)
	if resp.Provider != "mock" || resp.ModelID != "mock-text" {
		t.Fatalf("provider/model = %s/%s", resp.Provider, resp.ModelID)
	}
	if resp.Intent["model"].(map[string]any)["provider"] != "mock" {
		t.Fatalf("intent = %#v", resp.Intent)
	}
	if resp.RenderedPrompt != "write a short ad" {
		t.Fatalf("rendered prompt = %q", resp.RenderedPrompt)
	}
	if resp.RequestedByID != "account-123" {
		t.Fatalf("requested by id = %q", resp.RequestedByID)
	}
}
```

Add imports:

```go
import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)
```

- [ ] **Step 4: Run response test and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestRunJobResponseExposesIntentAndSummaries -count=1
```

Expected: FAIL because response helpers do not exist.

- [ ] **Step 5: Implement response DTOs and run result response**

In `apps/server/internal/api/run_handler.go`, add:

```go
type runNodeResponse struct {
	Node    db.MediaNode           `json:"node"`
	Job     generationJobResponse `json:"job"`
	Version db.ArtifactVersion     `json:"version,omitempty"`
}

type generationJobResponse struct {
	ID               string         `json:"id"`
	TargetNodeID     string         `json:"target_node_id"`
	OperationType    string         `json:"operation_type"`
	Provider         string         `json:"provider"`
	ModelID          string         `json:"model_id"`
	Intent           map[string]any `json:"intent"`
	RenderedPrompt   string         `json:"rendered_prompt"`
	ProviderRequest  map[string]any `json:"provider_request"`
	ProviderResponse map[string]any `json:"provider_response"`
	Status           string         `json:"status"`
	ErrorCode        string         `json:"error_code,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	RequestedByType  string         `json:"requested_by_type"`
	RequestedByID    string         `json:"requested_by_id,omitempty"`
}

func toGenerationJobResponse(job db.GenerationJob) generationJobResponse {
	return generationJobResponse{
		ID:               uuidToString(job.ID),
		TargetNodeID:     uuidToString(job.TargetNodeID),
		OperationType:    job.OperationType,
		Provider:         job.Provider,
		ModelID:          job.ModelID,
		Intent:           jsonObject(job.Intent),
		RenderedPrompt:   job.RenderedPrompt,
		ProviderRequest:  jsonObject(job.ProviderRequest),
		ProviderResponse: jsonObject(job.ProviderResponse),
		Status:           string(job.Status),
		ErrorCode:        textString(job.ErrorCode),
		ErrorMessage:     textString(job.ErrorMessage),
		RequestedByType:  job.RequestedByType,
		RequestedByID:    textString(job.RequestedByID),
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

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
```

Add imports:

```go
import (
	"context"
	"encoding/json"
	"errors"
)
```

- [ ] **Step 6: Update run handler to return job/version and preserve failure visibility**

Replace the service call block in `RunNode` with:

```go
	result, err := h.service.RunNode(ctx, nodeID, production.RequestedBy{Type: "user", ID: uuidToString(accountID)})
	if err != nil {
		latest, latestErr := h.queries.LatestGenerationJobByNode(ctx, nodeID)
		if latestErr == nil {
			c.JSON(statusForRunError(err), runNodeResponse{Job: toGenerationJobResponse(latest)})
			return
		}
		if errors.Is(err, production.ErrUnsupportedNodeType) {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
		writeError(c, consts.StatusInternalServerError, "failed to run node")
		return
	}
	c.JSON(consts.StatusOK, runNodeResponse{
		Node:    result.Node,
		Job:     toGenerationJobResponse(result.Job),
		Version: result.Version,
	})
```

Add helper:

```go
func statusForRunError(err error) int {
	if errors.Is(err, production.ErrUnsupportedNodeType) ||
		errors.Is(err, production.ErrProviderConfig) ||
		errors.Is(err, production.ErrProviderUnavailable) {
		return consts.StatusBadRequest
	}
	return consts.StatusInternalServerError
}
```

- [ ] **Step 7: Add job list endpoint**

Add method to `RunHandler`:

```go
func (h *RunHandler) ListNodeJobs(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	jobs, err := h.queries.ListGenerationJobsByNode(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list jobs")
		return
	}
	resp := make([]generationJobResponse, 0, len(jobs))
	for _, job := range jobs {
		resp = append(resp, toGenerationJobResponse(job))
	}
	c.JSON(consts.StatusOK, resp)
}
```

Wire route in `apps/server/cmd/server/main.go`:

```go
h.GET("/api/nodes/:id/jobs", authMiddleware, runHandler.ListNodeJobs)
```

- [ ] **Step 8: Run API tests and verify pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestRunJobResponse|TestNewRunHandler' -count=1
```

Expected: PASS.

---

### Task 8: Wire Registry From Config

**Files:**
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Replace direct mock provider construction**

Replace:

```go
productionService := production.NewService(pgPool, queries, production.MockTextProvider{})
```

with:

```go
providerRegistry := production.NewProviderRegistry(production.ProviderConfig{
	ProviderMode:     cfg.Production.ProviderMode,
	DefaultProvider:  cfg.Production.DefaultProvider,
	DefaultTextModel: cfg.Production.DefaultTextModel,
	Volcengine: production.VolcengineProviderConfig{
		APIKey:     cfg.Production.Volcengine.APIKey,
		BaseURL:    cfg.Production.Volcengine.BaseURL,
		TextModel:  cfg.Production.Volcengine.TextModel,
		ImageModel: cfg.Production.Volcengine.ImageModel,
		VideoModel: cfg.Production.Volcengine.VideoModel,
	},
})
productionService := production.NewService(pgPool, queries, providerRegistry)
```

- [ ] **Step 2: Run server build**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

---

### Task 9: Add M4.2 Smoke Script

**Files:**
- Create: `scripts/smoke-m4-2.sh`

- [ ] **Step 1: Create smoke script**

Create executable `scripts/smoke-m4-2.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

node <<'NODE'
const base = process.env.CLIPANVIL_API_BASE || `http://127.0.0.1:${process.env.CLIPANVIL_SERVER_PORT || "8888"}/api`;

async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) {
    const error = new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
    error.status = res.status;
    error.body = text;
    throw error;
  }
  return text ? JSON.parse(text) : null;
}

async function reqAllowError(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  return {status: res.status, body: text ? JSON.parse(text) : null};
}

const email = `m4-2-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.2 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({name: "M4.2 Smoke", mode: "studio"}),
});

const mockNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Mock intent",
    prompt: "Write a crisp product line",
    operation_type: "text_generation",
    model_provider: "mock",
    model_id: "mock-text",
    model_params: {temperature: 0.2},
    canvas_x: 20,
    canvas_y: 40,
  }),
});

const run = await req(`/nodes/${mockNode.id}/run`, {method: "POST", headers});
if (!run.node?.current_version_id || run.job?.status !== "succeeded") {
  throw new Error(`mock run did not succeed: ${JSON.stringify(run)}`);
}
if (run.job.intent.model.provider !== "mock" || run.job.intent.model.model_id !== "mock-text") {
  throw new Error(`unexpected intent model: ${JSON.stringify(run.job.intent.model)}`);
}
if (run.job.intent.params.temperature !== 0.2) {
  throw new Error(`unexpected intent params: ${JSON.stringify(run.job.intent.params)}`);
}
if (run.job.rendered_prompt !== "Write a crisp product line") {
  throw new Error(`unexpected rendered prompt: ${run.job.rendered_prompt}`);
}
if (run.job.provider_request.provider !== "mock" || run.job.provider_response.provider !== "mock") {
  throw new Error(`mock provider summaries missing: ${JSON.stringify(run.job)}`);
}

const jobs = await req(`/nodes/${mockNode.id}/jobs`, {headers});
if (jobs.length !== 1 || jobs[0].id !== run.job.id) {
  throw new Error(`job listing mismatch: ${JSON.stringify(jobs)}`);
}

const realNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Missing key",
    prompt: "This should not call a real provider",
    operation_type: "text_generation",
    model_provider: "volcengine",
    model_id: "doubao-seed-1-6-lite",
    model_params: {temperature: 0.1},
    canvas_x: 40,
    canvas_y: 60,
  }),
});

const failed = await reqAllowError(`/nodes/${realNode.id}/run`, {method: "POST", headers});
if (failed.status !== 400) {
  throw new Error(`expected missing key status 400, got ${failed.status}: ${JSON.stringify(failed.body)}`);
}
if (failed.body.job?.status !== "failed" || failed.body.job?.error_code !== "provider_config_error") {
  throw new Error(`missing key failure was not persisted: ${JSON.stringify(failed.body)}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  mockNodeId: mockNode.id,
  mockJobId: run.job.id,
  failedNodeId: realNode.id,
  failedJobId: failed.body.job.id,
}, null, 2));
NODE
```

- [ ] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/smoke-m4-2.sh
```

- [ ] **Step 3: Run smoke against a running local server**

Start the app with `./scripts/dev-start.sh` or a foreground backend matching the current worktree port, then run:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-2.sh
```

Expected: PASS and JSON output containing one succeeded mock job and one failed Volcengine missing-key job.

---

### Task 10: Full Verification And M4.2 Completion Record

**Files:**
- Modify: `docs/milestones/m4-shared-production-foundation.md`

- [ ] **Step 1: Run required verification**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all pass.

- [ ] **Step 2: Run M4.1 smoke for regression**

Run:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-1.sh
```

Expected: PASS after updating the script to read `run.node.current_version_id` if M4.2 changes the run response node.

- [ ] **Step 3: Run M4.2 smoke**

Run:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-2.sh
```

Expected: PASS.

- [ ] **Step 4: Record completion in milestone doc**

Under `### M4.2 GenerationIntent And Provider Bridge Skeleton` in `docs/milestones/m4-shared-production-foundation.md`, append:

```markdown
Completion record:

- Node runs are converted to the stable nested `GenerationIntent` before provider execution.
- Provider registry selects mock providers by default and supports a Volcengine adapter boundary behind environment config.
- Missing Volcengine API keys fail before any external call and persist a failed `generation_job`.
- Successful mock runs persist `intent`, `rendered_prompt`, provider request summary, provider response summary, and requested-by metadata.
- Studio API accepts production node fields needed by intent construction.
- Node job history is queryable for smoke tests and future UI.

Verification:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-1.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-2.sh
git diff --check
```
```

- [ ] **Step 5: Final self-review before handoff**

Check:

```bash
rg -n "MockTextProvider|model_provider|model_id" apps/server/internal/production apps/server/cmd/server/main.go
rg -n "PLACEHOLDER_MARKER_SHOULD_NOT_EXIST" docs/superpowers/plans/2026-06-18-m4-2-generation-intent-provider-bridge.md
git diff --check
```

Expected:

- No stale `MockTextProvider` references remain.
- Any `model_provider` / `model_id` usage in production code is only DB compatibility or config mapping; persisted intent uses nested `model`.
- No placeholder language remains in this plan.
- `git diff --check` passes.

---

## E2E Acceptance Gate

M4.2 is complete only when this entire gate passes:

1. A Studio Text Node can be created with `prompt`, `operation_type`, `model_provider`, `model_id`, and `model_params`.
2. Running that node with mock provider config returns a succeeded job and current version.
3. The latest `generation_job.intent` includes `workspace_id`, `target_node_id`, `output_type`, `operation_type`, `prompt_template`, nested `model`, `params`, and `requested_by`.
4. `generation_job.rendered_prompt`, `provider_request`, and `provider_response` are non-empty and inspectable through the job API.
5. Requesting `volcengine` with no API key returns a clear `400` and persists a failed job with `error_code=provider_config_error`.
6. No automated test requires external network access or provider credentials.
7. M4.1 smoke still passes.

## Execution Note

The user has already chosen inline development for M4 work. Implement this plan directly in the current session with `superpowers:executing-plans`, task by task, and run the acceptance gate before marking M4.2 complete.
