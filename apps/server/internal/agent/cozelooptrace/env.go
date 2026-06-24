package cozelooptrace

import (
	"bufio"
	"os"
	"strings"
)

const defaultAgentServiceName = "clipanvil-agent"

func LoadDotEnvFiles(paths ...string) {
	for _, path := range paths {
		loadDotEnv(path)
	}
}

func ConfigFromEnv() Config {
	authorization := envOrDefault("CLIPANVIL_COZELOOP_AUTHORIZATION", os.Getenv("CLIPANVIL_COZELOOP_PAT"))
	return Config{
		Endpoint:      envOrDefault("CLIPANVIL_COZELOOP_ENDPOINT", DefaultEndpoint),
		WorkspaceID:   strings.TrimSpace(os.Getenv("CLIPANVIL_COZELOOP_WORKSPACE_ID")),
		Authorization: authorization,
		ServiceName:   envOrDefault("CLIPANVIL_COZELOOP_SERVICE_NAME", defaultAgentServiceName),
	}
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
