package cozelooptrace

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/coze-dev/cozeloop-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

const (
	DefaultEndpoint = "http://localhost:19098"
	otlpTracePath   = "/v1/loop/opentelemetry/v1/traces"
)

var ErrMissingWorkspaceID = errors.New("cozeloop workspace id is required")
var ErrMissingAuthorization = errors.New("cozeloop authorization is required")

type Config struct {
	Endpoint      string
	WorkspaceID   string
	Authorization string
	ServiceName   string
	Timeout       time.Duration
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.WorkspaceID) == "" {
		return ErrMissingWorkspaceID
	}
	if strings.TrimSpace(c.Authorization) == "" {
		return ErrMissingAuthorization
	}
	_, err := c.OTLPEndpointURL()
	return err
}

func (c Config) OTLPEndpointURL() (string, error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("cozeloop endpoint must include scheme and host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = otlpTracePath
	} else if parsed.Path != otlpTracePath {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + otlpTracePath
	}
	return parsed.String(), nil
}

func (c Config) APIBaseURL() (string, error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("cozeloop endpoint must include scheme and host")
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func NewClient(config Config) (cozeloop.Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	apiBaseURL, err := config.APIBaseURL()
	if err != nil {
		return nil, err
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return cozeloop.NewClient(
		cozeloop.WithAPIBaseURL(apiBaseURL),
		cozeloop.WithWorkspaceID(strings.TrimSpace(config.WorkspaceID)),
		cozeloop.WithAPIToken(normalizeAPIToken(config.Authorization)),
		cozeloop.WithTimeout(timeout),
		cozeloop.WithUploadTimeout(timeout),
	)
}

func NewTracerProvider(ctx context.Context, config Config) (*sdktrace.TracerProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	endpointURL, err := config.OTLPEndpointURL()
	if err != nil {
		return nil, err
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpointURL),
		otlptracehttp.WithHeaders(map[string]string{
			"cozeloop-workspace-id": strings.TrimSpace(config.WorkspaceID),
			"Authorization":         normalizeAuthorization(config.Authorization),
		}),
		otlptracehttp.WithTimeout(timeout),
	)
	if err != nil {
		return nil, err
	}
	serviceName := strings.TrimSpace(config.ServiceName)
	if serviceName == "" {
		serviceName = "clipanvil-cozeloop-trace-smoke"
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		attribute.String("cozeloop.workspace_id", strings.TrimSpace(config.WorkspaceID)),
	))
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	), nil
}

func normalizeAPIToken(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[len("bearer "):])
	}
	return value
}

func normalizeAuthorization(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return value
	}
	return "Bearer " + value
}
