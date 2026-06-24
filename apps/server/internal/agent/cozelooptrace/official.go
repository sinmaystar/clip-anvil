package cozelooptrace

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	loopcallback "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/cozeloop-go"
	"go.opentelemetry.io/otel/attribute"
)

func NewOfficialCallbackHandler(client cozeloop.Client, serviceName string) callbacks.Handler {
	parser := newSanitizingDataParser(loopcallback.NewDefaultDataParser(true))
	inner := loopcallback.NewLoopHandler(
		client,
		loopcallback.WithAggrMessageOutput(true),
		loopcallback.WithCallbackDataParser(parser),
	)
	return officialCallbackHandler{
		client:      client,
		inner:       inner,
		serviceName: strings.TrimSpace(serviceName),
	}
}

type officialCallbackHandler struct {
	client      cozeloop.Client
	inner       callbacks.Handler
	serviceName string
}

func (h officialCallbackHandler) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	ctx = h.inner.OnStart(ctx, info, input)
	h.setContextTags(ctx)
	return ctx
}

func (h officialCallbackHandler) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	return h.inner.OnEnd(ctx, info, output)
}

func (h officialCallbackHandler) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	return h.inner.OnError(ctx, info, err)
}

func (h officialCallbackHandler) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	ctx = h.inner.OnStartWithStreamInput(ctx, info, input)
	h.setContextTags(ctx)
	return ctx
}

func (h officialCallbackHandler) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	return h.inner.OnEndWithStreamOutput(ctx, info, output)
}

func (h officialCallbackHandler) setContextTags(ctx context.Context) {
	if h.client == nil {
		return
	}
	span := h.client.GetSpanFromContext(ctx)
	if span == nil {
		return
	}
	if h.serviceName != "" {
		span.SetServiceName(ctx, h.serviceName)
	}
	tags := tagsFromAttributes(AttributesFromContext(ctx))
	if len(tags) > 0 {
		span.SetTags(ctx, tags)
	}
}

func tagsFromAttributes(attrs []attribute.KeyValue) map[string]interface{} {
	if len(attrs) == 0 {
		return nil
	}
	tags := make(map[string]interface{}, len(attrs))
	for _, attr := range attrs {
		if attr.Key == "" {
			continue
		}
		tags[string(attr.Key)] = attr.Value.AsInterface()
	}
	return tags
}

type sanitizingDataParser struct {
	inner loopcallback.CallbackDataParser
}

func newSanitizingDataParser(inner loopcallback.CallbackDataParser) loopcallback.CallbackDataParser {
	if inner == nil {
		inner = loopcallback.NewDefaultDataParser(true)
	}
	return sanitizingDataParser{inner: inner}
}

func (p sanitizingDataParser) ParseInput(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) map[string]any {
	return p.inner.ParseInput(ctx, info, sanitizeTraceValue(input))
}

func (p sanitizingDataParser) ParseOutput(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) map[string]any {
	return p.inner.ParseOutput(ctx, info, sanitizeTraceValue(output))
}

func (p sanitizingDataParser) ParseStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) map[string]any {
	return p.inner.ParseStreamInput(ctx, info, input)
}

func (p sanitizingDataParser) ParseStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) map[string]any {
	return p.inner.ParseStreamOutput(ctx, info, output)
}

func sanitizeTraceValue(value any) any {
	if value == nil {
		return nil
	}
	if canJSONMarshal(value) {
		return value
	}
	sanitized, ok := sanitizeReflectValue(reflect.ValueOf(value), make(map[reflectVisit]bool))
	if !ok {
		return nil
	}
	if canJSONMarshal(sanitized) {
		return sanitized
	}
	return fmt.Sprint(sanitized)
}

type reflectVisit struct {
	typ reflect.Type
	ptr uintptr
}

func sanitizeReflectValue(value reflect.Value, seen map[reflectVisit]bool) (any, bool) {
	if !value.IsValid() {
		return nil, true
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, true
		}
		if value.Kind() == reflect.Pointer {
			visit := reflectVisit{typ: value.Type(), ptr: value.Pointer()}
			if seen[visit] {
				return "<cycle>", true
			}
			seen[visit] = true
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return nil, false
	case reflect.Struct:
		if value.CanInterface() && canJSONMarshal(value.Interface()) {
			return value.Interface(), true
		}
		out := make(map[string]any)
		valueType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := valueType.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name, include := jsonFieldName(field)
			if !include {
				continue
			}
			sanitized, ok := sanitizeReflectValue(value.Field(i), seen)
			if !ok {
				continue
			}
			out[name] = sanitized
		}
		return out, true
	case reflect.Map:
		if value.IsNil() {
			return nil, true
		}
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			sanitized, ok := sanitizeReflectValue(iter.Value(), seen)
			if !ok {
				continue
			}
			out[fmt.Sprint(valueInterface(iter.Key()))] = sanitized
		}
		return out, true
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return nil, true
		}
		out := make([]any, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			sanitized, ok := sanitizeReflectValue(value.Index(i), seen)
			if !ok {
				continue
			}
			out = append(out, sanitized)
		}
		return out, true
	default:
		if value.CanInterface() {
			v := value.Interface()
			if canJSONMarshal(v) {
				return v, true
			}
			return fmt.Sprint(v), true
		}
		return fmt.Sprint(value), true
	}
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	if tag != "" {
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			return name, true
		}
	}
	return field.Name, true
}

func canJSONMarshal(value any) bool {
	_, err := json.Marshal(value)
	return err == nil
}

func valueInterface(value reflect.Value) any {
	if value.CanInterface() {
		return value.Interface()
	}
	return fmt.Sprint(value)
}
