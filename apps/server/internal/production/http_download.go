package production

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultProviderDownloadMaxBytes = 50 << 20

func downloadProviderAsset(ctx context.Context, client *http.Client, url string, maxBytes int64, allowedMIMEs map[string]bool) ([]byte, string, error) {
	if strings.TrimSpace(url) == "" {
		return nil, "", fmt.Errorf("%w: provider output url is empty", ErrProviderExecution)
	}
	if client == nil {
		client = http.DefaultClient
	}
	if maxBytes <= 0 {
		maxBytes = defaultProviderDownloadMaxBytes
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid provider output url", ErrProviderExecution)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%w: download provider output: %v", ErrProviderExecution, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("%w: download provider output status %d", ErrProviderExecution, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: read provider output: %v", ErrProviderExecution, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("%w: provider output exceeds max bytes", ErrProviderExecution)
	}
	mime := http.DetectContentType(data)
	if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
		mime = strings.Split(contentType, ";")[0]
	}
	if len(allowedMIMEs) > 0 && !allowedMIMEs[mime] {
		return nil, "", fmt.Errorf("%w: unsupported provider output mime %s", ErrProviderExecution, mime)
	}
	return data, mime, nil
}
