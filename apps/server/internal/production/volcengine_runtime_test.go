package production

import (
	"context"
	"testing"
	"time"
)

type fakeProductionRuntime struct {
	started bool
}

func (f *fakeProductionRuntime) Start(context.Context, ProductionJob, GenerationIntent) (<-chan ProductionEvent, error) {
	f.started = true
	events := make(chan ProductionEvent, 1)
	events <- ProductionEvent{Type: ProductionEventJobSucceeded, Output: ProductionOutput{TextContent: "ok"}}
	close(events)
	return events, nil
}

func TestVolcengineProductionRuntimeDelegatesNonVolcengineProvider(t *testing.T) {
	legacy := &fakeProductionRuntime{}
	runtime := VolcengineProductionRuntime{legacy: legacy}
	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OutputType: "text",
		Model:      ModelSpec{Provider: "mock", ModelID: "mock-text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if !legacy.started {
		t.Fatal("expected legacy runtime to start")
	}
}

func TestVolcengineProductionRuntimeRoutesAudioRuntime(t *testing.T) {
	audio := &fakeProductionRuntime{}
	runtime := VolcengineProductionRuntime{audio: audio}
	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OutputType: "audio",
		Model:      ModelSpec{Provider: "volcengine", ModelID: "seed-audio-1.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if !audio.started {
		t.Fatal("expected audio runtime to start")
	}
}

func TestNewVolcengineProductionRuntimePassesInputResolverToVideoRuntime(t *testing.T) {
	resolver := &fakeProviderInputResolver{url: "https://signed.example/input.png"}
	runtime := NewVolcengineProductionRuntime(VolcengineProviderConfig{}, nil, time.Second, time.Minute, nil, resolver)
	video, ok := runtime.video.(VolcengineVideoRuntime)
	if !ok {
		t.Fatalf("video runtime = %T", runtime.video)
	}
	if video.inputResolver != resolver {
		t.Fatal("expected video runtime to receive input resolver")
	}
}

func TestNewVolcengineProductionRuntimePassesInputResolverToImageRuntime(t *testing.T) {
	resolver := &fakeProviderInputResolver{url: "https://signed.example/input.png"}
	runtime := NewVolcengineProductionRuntime(VolcengineProviderConfig{}, nil, time.Second, time.Minute, nil, resolver)
	image, ok := runtime.image.(VolcengineImageRuntime)
	if !ok {
		t.Fatalf("image runtime = %T", runtime.image)
	}
	if image.inputResolver != resolver {
		t.Fatal("expected image runtime to receive input resolver")
	}
}
