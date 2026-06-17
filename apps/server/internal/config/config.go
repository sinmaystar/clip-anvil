package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	MinIO    MinIOConfig
	JWT      JWTConfig
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
	Endpoint  string
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	UseSSL    bool   `mapstructure:"use_ssl"`
}

type JWTConfig struct {
	Secret      string
	ExpireHours int `mapstructure:"expire_hours"`
}

func Load() (*Config, error) {
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

func bindEnv(v *viper.Viper) error {
	keys := []string{
		"server.port",
		"postgres.dsn",
		"redis.addr",
		"minio.endpoint",
		"minio.access_key",
		"minio.secret_key",
		"minio.use_ssl",
		"jwt.secret",
		"jwt.expire_hours",
	}
	for _, key := range keys {
		if err := v.BindEnv(key); err != nil {
			return err
		}
	}
	return nil
}
