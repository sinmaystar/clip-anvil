package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig
	Postgres   PostgresConfig
	Redis      RedisConfig
	MinIO      MinIOConfig
	JWT        JWTConfig
	Sandbox    SandboxConfig
	Production ProductionConfig
	Agent      AgentConfig
}

type ServerConfig struct {
	Port int
}

type PostgresConfig struct {
	DSN string
}

type RedisConfig struct {
	Addr string
}

type MinIOConfig struct {
	Endpoint        string
	SandboxEndpoint string `mapstructure:"sandbox_endpoint"`
	AccessKey       string `mapstructure:"access_key"`
	SecretKey       string `mapstructure:"secret_key"`
	UseSSL          bool   `mapstructure:"use_ssl"`
}

type JWTConfig struct {
	Secret      string
	ExpireHours int `mapstructure:"expire_hours"`
}

type SandboxConfig struct {
	Endpoint       string
	APIKey         string `mapstructure:"api_key"`
	Image          string
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
	Workdir        string
	UseServerProxy bool                  `mapstructure:"use_server_proxy"`
	ResourceLimits SandboxResourceLimits `mapstructure:"resource_limits"`
}

type SandboxResourceLimits struct {
	CPU    string
	Memory string
}

type AgentConfig struct {
	ProducerMaxToolCalls int `mapstructure:"producer_max_tool_calls"`
	ToolTimeoutSeconds   int `mapstructure:"tool_timeout_seconds"`
}

type ProductionConfig struct {
	ProviderMode                string `mapstructure:"provider_mode"`
	DefaultProvider             string `mapstructure:"default_provider"`
	DefaultTextModel            string `mapstructure:"default_text_model"`
	WorkerConcurrency           int    `mapstructure:"worker_concurrency"`
	ProviderPollIntervalSeconds int    `mapstructure:"provider_poll_interval_seconds"`
	ProviderMaxPollSeconds      int    `mapstructure:"provider_max_poll_seconds"`
	Volcengine                  VolcengineConfig
}

type VolcengineConfig struct {
	APIKey                  string `mapstructure:"api_key"`
	BaseURL                 string `mapstructure:"base_url"`
	Region                  string `mapstructure:"region"`
	TextModel               string `mapstructure:"text_model"`
	ImageModel              string `mapstructure:"image_model"`
	VideoModel              string `mapstructure:"video_model"`
	VideoResolutionOverride string `mapstructure:"video_resolution_override"`
	AudioModel              string `mapstructure:"audio_model"`
	TOS                     TOSConfig
}

type TOSConfig struct {
	AccessKeyID         string `mapstructure:"access_key_id"`
	SecretAccessKey     string `mapstructure:"secret_access_key"`
	Bucket              string `mapstructure:"bucket"`
	Endpoint            string `mapstructure:"endpoint"`
	Region              string `mapstructure:"region"`
	PublicBaseURL       string `mapstructure:"public_base_url"`
	SignedURLTTLSeconds int    `mapstructure:"signed_url_ttl_seconds"`
}

func Load() (*Config, error) {
	_ = loadDotEnv(".env")
	_ = loadDotEnv("../../.env")

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
	if cfg.Agent.ProducerMaxToolCalls <= 0 {
		cfg.Agent.ProducerMaxToolCalls = 1000
	}
	if cfg.Agent.ToolTimeoutSeconds <= 0 {
		cfg.Agent.ToolTimeoutSeconds = 300
	}
	return &cfg, nil
}

func bindEnv(v *viper.Viper) error {
	keys := []string{
		"server.port",
		"postgres.dsn",
		"redis.addr",
		"minio.endpoint",
		"minio.sandbox_endpoint",
		"minio.access_key",
		"minio.secret_key",
		"minio.use_ssl",
		"jwt.secret",
		"jwt.expire_hours",
		"sandbox.endpoint",
		"sandbox.api_key",
		"sandbox.image",
		"sandbox.timeout_seconds",
		"sandbox.workdir",
		"sandbox.use_server_proxy",
		"sandbox.resource_limits.cpu",
		"sandbox.resource_limits.memory",
		"agent.producer_max_tool_calls",
		"agent.tool_timeout_seconds",
		"production.provider_mode",
		"production.default_provider",
		"production.default_text_model",
		"production.worker_concurrency",
		"production.provider_poll_interval_seconds",
		"production.provider_max_poll_seconds",
		"production.volcengine.api_key",
		"production.volcengine.base_url",
		"production.volcengine.region",
		"production.volcengine.text_model",
		"production.volcengine.image_model",
		"production.volcengine.video_model",
		"production.volcengine.video_resolution_override",
		"production.volcengine.audio_model",
		"production.volcengine.tos.access_key_id",
		"production.volcengine.tos.secret_access_key",
		"production.volcengine.tos.bucket",
		"production.volcengine.tos.endpoint",
		"production.volcengine.tos.region",
		"production.volcengine.tos.public_base_url",
		"production.volcengine.tos.signed_url_ttl_seconds",
	}
	for _, key := range keys {
		if err := v.BindEnv(key); err != nil {
			return err
		}
	}
	return nil
}

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
