package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"strings"
)

const (
	AssetsDir  = "/workspace/assets"
	ScriptsDir = "/workspace/scripts"
	TmpDir     = "/workspace/tmp"
	OutputDir  = "/workspace/output"
)

type WorkspaceManifest struct {
	WorkspaceID string          `json:"workspace_id"`
	AssetsDir   string          `json:"assets_dir"`
	OutputDir   string          `json:"output_dir"`
	Assets      []ManifestAsset `json:"assets"`
}

type ManifestAsset struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Mime  string `json:"mime"`
	Path  string `json:"path"`
	Title string `json:"title"`
}

type AssetForPreload struct {
	ID       string
	Type     string
	Mime     string
	Title    string
	Filename string
	Reader   io.Reader
}

func SafeAssetName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "asset.bin"
	}
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	if b.Len() == 0 {
		return "asset.bin"
	}
	return b.String()
}

func EnsureWorkspaceLayout(ctx context.Context, client Client, sandboxID string) error {
	_, err := client.Exec(ctx, sandboxID, ExecRequest{
		Command:        "mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output",
		Cwd:            DefaultWorkdir,
		TimeoutSeconds: 30,
	})
	return err
}

func WriteManifest(ctx context.Context, client Client, sandboxID string, manifest WorkspaceManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return client.Upload(ctx, sandboxID, "/workspace/manifest.json", strings.NewReader(string(data)))
}

func PrepareWorkspaceFiles(ctx context.Context, client Client, sandboxID string, workspaceID string, assets []AssetForPreload) (WorkspaceManifest, error) {
	if err := EnsureWorkspaceLayout(ctx, client, sandboxID); err != nil {
		return WorkspaceManifest{}, err
	}
	manifest := WorkspaceManifest{
		WorkspaceID: workspaceID,
		AssetsDir:   AssetsDir,
		OutputDir:   OutputDir,
		Assets:      make([]ManifestAsset, 0, len(assets)),
	}
	for _, asset := range assets {
		path := AssetsDir + "/" + asset.ID + "-" + SafeAssetName(asset.Filename)
		if err := client.Upload(ctx, sandboxID, path, asset.Reader); err != nil {
			return WorkspaceManifest{}, err
		}
		manifest.Assets = append(manifest.Assets, ManifestAsset{
			ID:    asset.ID,
			Type:  asset.Type,
			Mime:  asset.Mime,
			Path:  path,
			Title: asset.Title,
		})
	}
	if err := WriteManifest(ctx, client, sandboxID, manifest); err != nil {
		return WorkspaceManifest{}, err
	}
	return manifest, nil
}
