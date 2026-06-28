package production

import (
	"context"
	"encoding/base64"
	"fmt"
)

type MockProvider struct{}

var onePixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

var mockMP4 = mustDecodeBase64("AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDEAAAAIZnJlZQAAAv1tZGF0AAACrgYF//+q3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NSByMzIyMiBiMzU2MDVhIC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyNSAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTMgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDExMyBtZT1oZXggc3VibWU9NyBwc3k9MSBwc3lfcmQ9MS4wMDowLjAwIG1peGVkX3JlZj0xIG1lX3JhbmdlPTE2IGNocm9tYV9tZT0xIHRyZWxsaXM9MSA4eDhkY3Q9MSBjcW09MCBkZWFkem9uZT0yMSwxMSBmYXN0X3Bza2lwPTEgY2hyb21hX3FwX29mZnNldD0tMiB0aHJlYWRzPTEgbG9va2FoZWFkX3RocmVhZHM9MSBzbGljZWRfdGhyZWFkcz0wIG5yPTAgZGVjaW1hdGU9MSBpbnRlcmxhY2VkPTAgYmx1cmF5X2NvbXBhdD0wIGNvbnN0cmFpbmVkX2ludHJhPTAgYmZyYW1lcz0zIGJfcHlyYW1pZD0yIGJfYWRhcHQ9MSBiX2JpYXM9MCBkaXJlY3Q9MSB3ZWlnaHRiPTEgb3Blbl9nb3A9MCB3ZWlnaHRwPTIga2V5aW50PTI1MCBrZXlpbnRfbWluPTI1IHNjZW5lY3V0PTQwIGludHJhX3JlZnJlc2g9MCByY19sb29rYWhlYWQ9NDAgcmM9Y3JmIG1idHJlZT0xIGNyZj0yMy4wIHFjb21wPTAuNjAgcXBtaW49MCBxcG1heD02OSBxcHN0ZXA9NCBpcF9yYXRpbz0xLjQwIGFxPTE6MS4wMACAAAAAD2WIhAAz//727L4FNhTIwQAAAAhBmiRsQr/+wAAAAAhBnkJ4hf/BgQAAAAgBnmF0Qr/EgAAAAAgBnmNqQr/EgQAAA3Vtb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAyAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAACn3RyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAyAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAEAAAABAAAAAAACRlZHRzAAAAHGVsc3QAAAAAAAAAAQAAAMgAAAQAAAEAAAAAAhdtZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAADIAAAAKAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAHCbWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAABgnN0YmwAAAC+c3RzZAAAAAAAAAABAAAArmF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAEAAQAEgAAABIAAAAAAAAAAEVTGF2YzYyLjI4LjEwMSBsaWJ4MjY0AAAAAAAAAAAAAAAY//8AAAA0YXZjQwFkAAr/4QAXZ2QACqzZXsBEAAADAAQAAAMAyDxIllgBAAZo6+PLIsD9+PgAAAAAEHBhc3AAAAABAAAAAQAAABRidHJ0AAAAAAAAdkgAAAAAAAAAGHN0dHMAAAAAAAAAAQAAAAUAAAIAAAAAFHN0c3MAAAAAAAAAAQAAAAEAAAA4Y3R0cwAAAAAAAAAFAAAAAQAABAAAAAABAAAKAAAAAAEAAAQAAAAAAQAAAAAAAAABAAACAAAAABxzdHNjAAAAAAAAAAEAAAABAAAABQAAAAEAAAAoc3RzegAAAAAAAAAAAAAABQAAAsUAAAAMAAAADAAAAAwAAAAMAAAAFHN0Y28AAAAAAAAAAQAAADAAAABidWR0YQAAAFptZXRhAAAAAAAAACFoZGxyAAAAAAAAAABtZGlyYXBwbAAAAAAAAAAAAAAAAC1pbHN0AAAAJal0b28AAAAdZGF0YQAAAAEAAAAATGF2ZjYyLjEyLjEwMQ==")

var mockWAV = []byte{
	'R', 'I', 'F', 'F', 36, 0, 0, 0, 'W', 'A', 'V', 'E',
	'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0,
	0x40, 0x1f, 0, 0, 0x80, 0x3e, 0, 0, 2, 0, 16, 0,
	'd', 'a', 't', 'a', 0, 0, 0, 0,
}

func (MockProvider) Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	select {
	case <-ctx.Done():
		return ProviderResult{}, ctx.Err()
	default:
	}

	rendered := intent.EffectivePrompt()
	if rendered == "" {
		rendered = "empty prompt"
	}
	if shouldMockFail(intent.Params) {
		return ProviderResult{}, fmt.Errorf("%w: mock provider failure", ErrProviderExecution)
	}
	text := fmt.Sprintf("[mock:%s] %s", intent.Model.ModelID, rendered)
	request := map[string]any{
		"provider":       intent.Model.Provider,
		"model_id":       intent.Model.ModelID,
		"operation_type": intent.OperationType,
		"prompt":         rendered,
		"params":         intent.Params,
	}
	response := map[string]any{
		"provider": "mock",
		"text":     text,
	}
	switch intent.OutputType {
	case "image":
		return ProviderResult{
			RenderedPrompt:   rendered,
			AssetContent:     onePixelPNG,
			AssetMIME:        "image/png",
			AssetMetadata:    map[string]any{"mock": true},
			ProviderRequest:  request,
			ProviderResponse: response,
		}, nil
	case "video":
		return ProviderResult{
			RenderedPrompt:   rendered,
			AssetContent:     mockMP4,
			AssetMIME:        "video/mp4",
			AssetMetadata:    map[string]any{"mock": true},
			ProviderRequest:  request,
			ProviderResponse: response,
		}, nil
	case "audio":
		return ProviderResult{
			RenderedPrompt:   rendered,
			AssetContent:     mockWAV,
			AssetMIME:        "audio/wav",
			AssetMetadata:    map[string]any{"mock": true},
			ProviderRequest:  request,
			ProviderResponse: response,
		}, nil
	default:
		return ProviderResult{
			RenderedPrompt:   rendered,
			TextContent:      text,
			ProviderRequest:  request,
			ProviderResponse: response,
		}, nil
	}
}

func shouldMockFail(params map[string]any) bool {
	value, ok := params["mock_fail"]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func mustDecodeBase64(value string) []byte {
	out, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return out
}
