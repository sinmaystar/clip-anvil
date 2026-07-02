package sandbox

import (
	"context"
	"io"
	"net/url"
	pathpkg "path"
	"strings"
	"time"

	osb "github.com/alibaba/OpenSandbox/sdks/sandbox/go"

	"github.com/sinmaystar/clip-anvil/internal/config"
)

type CreateRequest struct {
	Image          string
	VolumeName     string
	MountPath      string
	TimeoutSeconds int
	ResourceCPU    string
	ResourceMemory string
}

type SandboxInfo struct {
	ID    string
	State string
}

type ExecRequest struct {
	Command        string
	Cwd            string
	TimeoutSeconds int
}

type ExecResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMS int64
	Truncated  bool
}

type FileInfo struct {
	Path      string
	SizeBytes int64
	Mime      string
}

type Client interface {
	Create(ctx context.Context, req CreateRequest) (SandboxInfo, error)
	Get(ctx context.Context, sandboxID string) (SandboxInfo, error)
	Ping(ctx context.Context, sandboxID string) error
	Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error)
	Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error
	Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error)
	Delete(ctx context.Context, sandboxID string) error
}

type OpenSandboxClient struct {
	cfg        config.SandboxConfig
	connection osb.ConnectionConfig
}

func NewOpenSandboxClient(cfg config.SandboxConfig) *OpenSandboxClient {
	return &OpenSandboxClient{
		cfg:        cfg,
		connection: connectionConfig(cfg),
	}
}

func (c *OpenSandboxClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	timeout := req.TimeoutSeconds
	createVolume := true
	deleteVolume := false
	sb, err := osb.CreateSandbox(ctx, c.connection, osb.SandboxCreateOptions{
		Image:          req.Image,
		TimeoutSeconds: &timeout,
		ResourceLimits: osb.ResourceLimits{
			"cpu":    req.ResourceCPU,
			"memory": req.ResourceMemory,
		},
		Volumes: []osb.Volume{{
			Name:      "workspace",
			MountPath: req.MountPath,
			PVC: &osb.PVC{
				ClaimName:                  req.VolumeName,
				CreateIfNotExists:          &createVolume,
				DeleteOnSandboxTermination: &deleteVolume,
			},
		}},
	})
	if err != nil {
		return SandboxInfo{}, err
	}
	return SandboxInfo{ID: sb.ID(), State: string(osb.StateRunning)}, nil
}

func (c *OpenSandboxClient) Get(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	sb, err := osb.ConnectSandbox(ctx, c.connection, sandboxID)
	if err != nil {
		return SandboxInfo{}, err
	}
	info, err := sb.GetInfo(ctx)
	if err != nil {
		return SandboxInfo{}, err
	}
	return SandboxInfo{ID: info.ID, State: string(info.Status.State)}, nil
}

func (c *OpenSandboxClient) Ping(ctx context.Context, sandboxID string) error {
	sb, err := osb.ConnectSandbox(ctx, c.connection, sandboxID)
	if err != nil {
		return err
	}
	return sb.Ping(ctx)
}

func (c *OpenSandboxClient) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error) {
	start := time.Now()
	sb, err := osb.ConnectSandbox(ctx, c.connection, sandboxID)
	if err != nil {
		return ExecResult{}, err
	}
	exec, err := sb.RunCommandWithOpts(ctx, osb.RunCommandRequest{
		Command: req.Command,
		Cwd:     req.Cwd,
		Timeout: execTimeoutMillis(req.TimeoutSeconds),
	}, nil)
	if err != nil {
		return ExecResult{}, err
	}
	exitCode := 0
	if exec.ExitCode != nil {
		exitCode = *exec.ExitCode
	}
	return ExecResult{
		ExitCode:   exitCode,
		Stdout:     outputText(exec.Stdout),
		Stderr:     outputText(exec.Stderr),
		DurationMS: time.Since(start).Milliseconds(),
	}, nil
}

func (c *OpenSandboxClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	sb, err := osb.ConnectSandbox(ctx, c.connection, sandboxID)
	if err != nil {
		return err
	}
	return sb.UploadFile(ctx, r, uploadFileOptions(path))
}

func (c *OpenSandboxClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error) {
	sb, err := osb.ConnectSandbox(ctx, c.connection, sandboxID)
	if err != nil {
		return nil, FileInfo{}, err
	}
	reader, err := sb.DownloadFile(ctx, path, "")
	if err != nil {
		return nil, FileInfo{}, err
	}
	info := FileInfo{Path: path, SizeBytes: -1}
	if entries, err := sb.GetFileInfo(ctx, path); err == nil {
		if file, ok := entries[path]; ok {
			info.SizeBytes = file.Size
		}
	}
	return reader, info, nil
}

func (c *OpenSandboxClient) Delete(ctx context.Context, sandboxID string) error {
	sb, err := osb.ConnectSandbox(ctx, c.connection, sandboxID)
	if err != nil {
		return err
	}
	return sb.Kill(ctx)
}

func connectionConfig(cfg config.SandboxConfig) osb.ConnectionConfig {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, "/v1")
	requestTimeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return osb.ConnectionConfig{
			Domain:         endpoint,
			APIKey:         cfg.APIKey,
			UseServerProxy: cfg.UseServerProxy,
			RequestTimeout: requestTimeout,
		}
	}
	return osb.ConnectionConfig{
		Domain:         parsed.Host,
		Protocol:       parsed.Scheme,
		APIKey:         cfg.APIKey,
		UseServerProxy: cfg.UseServerProxy,
		RequestTimeout: requestTimeout,
		EndpointHostRewrite: map[string]string{
			"host.docker.internal": "localhost",
		},
	}
}

func outputText(messages []osb.OutputMessage) string {
	var b strings.Builder
	for i, message := range messages {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(message.Text)
	}
	return b.String()
}

func uploadFileOptions(remotePath string) osb.UploadFileOptions {
	fileName := pathpkg.Base(remotePath)
	if fileName == "." || fileName == "/" || fileName == "" {
		fileName = "file"
	}
	return osb.UploadFileOptions{
		FileName: fileName,
		Metadata: osb.FileMetadata{
			Path: remotePath,
		},
	}
}
