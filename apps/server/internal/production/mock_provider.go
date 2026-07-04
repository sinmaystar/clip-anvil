package production

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
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
		audio := mockWAV(intent)
		return ProviderResult{
			RenderedPrompt:   rendered,
			AssetContent:     audio,
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

func mockWAV(intent GenerationIntent) []byte {
	sampleRate := mockIntParam(intent.Params, "sample_rate", 48000)
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	durationSec := mockFloatParam(intent.Params, "duration_sec", 8)
	if durationSec <= 0 {
		durationSec = 8
	}
	frequency := 440.0
	if strings.EqualFold(intent.Semantic.ArtifactKind, "bgm_audio") {
		frequency = 220.0
	}
	samples := int(float64(sampleRate) * durationSec)
	dataSize := samples * 2
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	for i := range samples {
		t := float64(i) / float64(sampleRate)
		envelope := 0.22
		if i < sampleRate/20 {
			envelope *= float64(i) / float64(sampleRate/20)
		}
		if samples-i < sampleRate/20 {
			envelope *= float64(samples-i) / float64(sampleRate/20)
		}
		value := int16(math.Sin(2*math.Pi*frequency*t) * envelope * math.MaxInt16)
		_ = binary.Write(&buf, binary.LittleEndian, value)
	}
	return buf.Bytes()
}

func mockIntParam(params map[string]any, key string, fallback int) int {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return fallback
	}
}

func mockFloatParam(params map[string]any, key string, fallback float64) float64 {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return fallback
	}
}

func mustDecodeBase64(value string) []byte {
	out, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return out
}
