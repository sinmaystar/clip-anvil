package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

const (
	AgentRemotionDir                    = "/workspace/agent-remotion"
	AgentRemotionPropsFile              = "props.json"
	DefaultAgentRemotionMaxFiles        = 16
	DefaultAgentRemotionMaxBytes        = 512 << 10
	DefaultAgentRemotionMaxFileBytes    = 128 << 10
	defaultAgentRemotionSourceEntryName = "files"
)

type AgentRemotionFile struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SizeBytes int64  `json:"size_bytes"`
}

type AgentRemotionWorkspaceInput struct {
	RendererArtifactID string
	AttemptNo          int32
	Files              map[string]string
	PropsJSON          []byte
}

type AgentRemotionSnapshot struct {
	WorkspaceDir   string              `json:"workspace_dir"`
	Files          []AgentRemotionFile `json:"files"`
	PropsJSON      []byte              `json:"props_json"`
	SourceHash     string              `json:"source_hash"`
	PropsHash      string              `json:"props_hash"`
	SourceSnapshot map[string]any      `json:"source_snapshot"`
}

func AgentRemotionAttemptDir(rendererArtifactID string, attemptNo int32) (string, error) {
	rendererArtifactID = strings.TrimSpace(rendererArtifactID)
	if !isSafeAgentRemotionSegment(rendererArtifactID) {
		return "", errors.New("renderer_artifact_id must be a safe path segment")
	}
	if attemptNo <= 0 {
		return "", errors.New("attempt_no must be > 0")
	}
	return AgentRemotionDir + "/" + rendererArtifactID + "/" + strconv.Itoa(int(attemptNo)), nil
}

func NormalizeAgentRemotionRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("path is required")
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, "..") {
		return "", errors.New("path must be a safe relative renderer file path")
	}
	clean := path.Clean(value)
	if clean == "." || clean != value {
		return "", errors.New("path must be normalized")
	}
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", errors.New("path contains an unsafe segment")
		}
		if part == "node_modules" || part == ".git" {
			return "", errors.New("path contains a forbidden segment")
		}
	}
	base := path.Base(clean)
	if base == AgentRemotionPropsFile || base == "package.json" {
		return "", errors.New("path is reserved")
	}
	if ext := path.Ext(clean); ext != ".ts" && ext != ".tsx" && ext != ".json" {
		return "", errors.New("renderer file must be .ts, .tsx, or .json")
	}
	return clean, nil
}

func BuildAgentRemotionSnapshot(workspaceDir string, files map[string]string, propsJSON []byte) (AgentRemotionSnapshot, error) {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" || !strings.HasPrefix(workspaceDir, AgentRemotionDir+"/") {
		return AgentRemotionSnapshot{}, errors.New("workspace_dir must be inside /workspace/agent-remotion")
	}
	if len(files) == 0 {
		return AgentRemotionSnapshot{}, errors.New("at least one renderer file is required")
	}
	if len(files) > DefaultAgentRemotionMaxFiles {
		return AgentRemotionSnapshot{}, fmt.Errorf("too many renderer files: %d > %d", len(files), DefaultAgentRemotionMaxFiles)
	}
	if len(propsJSON) > DefaultAgentRemotionMaxFileBytes {
		return AgentRemotionSnapshot{}, errors.New("props.json exceeds max file size")
	}

	paths := make([]string, 0, len(files))
	for name := range files {
		normalized, err := NormalizeAgentRemotionRelativePath(name)
		if err != nil {
			return AgentRemotionSnapshot{}, fmt.Errorf("%s: %w", name, err)
		}
		paths = append(paths, normalized)
	}
	sort.Strings(paths)

	totalBytes := len(propsJSON)
	fileList := make([]AgentRemotionFile, 0, len(paths))
	filesByPath := make(map[string]string, len(paths))
	for _, name := range paths {
		content := files[name]
		size := len([]byte(content))
		if size > DefaultAgentRemotionMaxFileBytes {
			return AgentRemotionSnapshot{}, fmt.Errorf("%s exceeds max file size", name)
		}
		totalBytes += size
		if totalBytes > DefaultAgentRemotionMaxBytes {
			return AgentRemotionSnapshot{}, errors.New("renderer snapshot exceeds max total size")
		}
		filesByPath[name] = content
		fileList = append(fileList, AgentRemotionFile{
			Path:      name,
			Content:   content,
			SizeBytes: int64(size),
		})
	}

	sourceSnapshot := map[string]any{
		defaultAgentRemotionSourceEntryName: filesByPath,
		"file_count":                        len(filesByPath),
	}
	sourceBytes, err := json.Marshal(sourceSnapshot)
	if err != nil {
		return AgentRemotionSnapshot{}, err
	}
	return AgentRemotionSnapshot{
		WorkspaceDir:   workspaceDir,
		Files:          fileList,
		PropsJSON:      append([]byte(nil), propsJSON...),
		SourceHash:     hashAgentRemotionBytes(sourceBytes),
		PropsHash:      hashAgentRemotionBytes(propsJSON),
		SourceSnapshot: sourceSnapshot,
	}, nil
}

func WriteAgentRemotionAttemptWorkspace(ctx context.Context, client Client, sandboxID string, input AgentRemotionWorkspaceInput) (AgentRemotionSnapshot, error) {
	if client == nil {
		return AgentRemotionSnapshot{}, errors.New("sandbox client is required")
	}
	workspaceDir, err := AgentRemotionAttemptDir(input.RendererArtifactID, input.AttemptNo)
	if err != nil {
		return AgentRemotionSnapshot{}, err
	}
	snapshot, err := BuildAgentRemotionSnapshot(workspaceDir, input.Files, input.PropsJSON)
	if err != nil {
		return AgentRemotionSnapshot{}, err
	}
	dirs := map[string]struct{}{workspaceDir: {}}
	for _, file := range snapshot.Files {
		dirs[path.Dir(workspaceDir+"/"+file.Path)] = struct{}{}
	}
	mkdirArgs := make([]string, 0, len(dirs))
	for dir := range dirs {
		mkdirArgs = append(mkdirArgs, shellQuote(dir))
	}
	sort.Strings(mkdirArgs)
	if _, err := client.Exec(ctx, sandboxID, ExecRequest{
		Command:        "mkdir -p " + strings.Join(mkdirArgs, " "),
		Cwd:            DefaultWorkdir,
		TimeoutSeconds: 30,
	}); err != nil {
		return AgentRemotionSnapshot{}, err
	}
	for _, file := range snapshot.Files {
		if err := client.Upload(ctx, sandboxID, workspaceDir+"/"+file.Path, strings.NewReader(file.Content)); err != nil {
			return AgentRemotionSnapshot{}, err
		}
	}
	if err := client.Upload(ctx, sandboxID, workspaceDir+"/"+AgentRemotionPropsFile, strings.NewReader(string(snapshot.PropsJSON))); err != nil {
		return AgentRemotionSnapshot{}, err
	}
	return snapshot, nil
}

func ReadAgentRemotionAttemptWorkspace(ctx context.Context, client Client, sandboxID string, workspaceDir string) (AgentRemotionSnapshot, error) {
	if client == nil {
		return AgentRemotionSnapshot{}, errors.New("sandbox client is required")
	}
	workspaceDir = path.Clean(strings.TrimSpace(workspaceDir))
	if workspaceDir == "." || !strings.HasPrefix(workspaceDir, AgentRemotionDir+"/") {
		return AgentRemotionSnapshot{}, errors.New("workspace_dir must be inside /workspace/agent-remotion")
	}
	result, err := client.Exec(ctx, sandboxID, ExecRequest{
		Command:        "find " + shellQuote(workspaceDir) + " -maxdepth 3 -type f | sort",
		Cwd:            DefaultWorkdir,
		TimeoutSeconds: 30,
	})
	if err != nil {
		return AgentRemotionSnapshot{}, err
	}
	if result.ExitCode != 0 {
		return AgentRemotionSnapshot{}, fmt.Errorf("list attempt workspace failed: %s", strings.TrimSpace(result.Stderr))
	}
	files := map[string]string{}
	var propsJSON []byte
	for _, found := range strings.Split(result.Stdout, "\n") {
		found = strings.TrimSpace(found)
		if found == "" {
			continue
		}
		clean := path.Clean(found)
		if !strings.HasPrefix(clean, workspaceDir+"/") {
			return AgentRemotionSnapshot{}, fmt.Errorf("attempt file escaped workspace: %s", found)
		}
		rel := strings.TrimPrefix(clean, workspaceDir+"/")
		reader, _, err := client.Download(ctx, sandboxID, clean)
		if err != nil {
			return AgentRemotionSnapshot{}, err
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return AgentRemotionSnapshot{}, readErr
		}
		if closeErr != nil {
			return AgentRemotionSnapshot{}, closeErr
		}
		if rel == AgentRemotionPropsFile {
			propsJSON = data
			continue
		}
		files[rel] = string(data)
	}
	return BuildAgentRemotionSnapshot(workspaceDir, files, propsJSON)
}

func isSafeAgentRemotionSegment(value string) bool {
	if value == "" || strings.HasPrefix(value, ".") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func hashAgentRemotionBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
