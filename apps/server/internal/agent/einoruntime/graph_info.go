package einoruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cloudwego/eino/compose"
)

type GraphInfoRegistry struct {
	mu     sync.Mutex
	graphs map[string]*compose.GraphInfo
}

func NewGraphInfoRegistry() *GraphInfoRegistry {
	return &GraphInfoRegistry{graphs: map[string]*compose.GraphInfo{}}
}

func (r *GraphInfoRegistry) CompileCallback() compose.GraphCompileCallback {
	return r
}

func (r *GraphInfoRegistry) OnFinish(_ context.Context, info *compose.GraphInfo) {
	if r == nil || info == nil || strings.TrimSpace(info.Name) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.graphs == nil {
		r.graphs = map[string]*compose.GraphInfo{}
	}
	r.graphs[info.Name] = info
}

func (r *GraphInfoRegistry) Get(graphName string) (*compose.GraphInfo, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	info, ok := r.graphs[graphName]
	return info, ok
}

func (r *GraphInfoRegistry) Mermaid(graphName string) (string, error) {
	info, ok := r.Get(graphName)
	if !ok {
		return "", fmt.Errorf("graph info not found: %s", graphName)
	}
	nodes := graphNodeNames(info)
	var builder strings.Builder
	builder.WriteString("flowchart TD\n")
	for _, node := range nodes {
		fmt.Fprintf(&builder, "  %s[\"%s\"]\n", mermaidID(node), escapeMermaidLabel(mermaidLabel(node)))
	}
	for _, from := range sortedEdgeKeys(info.Edges) {
		toNodes := append([]string(nil), info.Edges[from]...)
		sort.Strings(toNodes)
		for _, to := range toNodes {
			fmt.Fprintf(&builder, "  %s --> %s\n", mermaidID(from), mermaidID(to))
		}
	}
	return strings.TrimRight(builder.String(), "\n"), nil
}

func (r *GraphInfoRegistry) JSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(map[string]any{})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	payload := map[string]any{}
	for _, name := range sortedGraphNames(r.graphs) {
		info := r.graphs[name]
		payload[name] = map[string]any{
			"name":  info.Name,
			"nodes": graphNodeNames(info),
			"edges": sortedEdges(info.Edges),
		}
	}
	return json.MarshalIndent(payload, "", "  ")
}

func graphNodeNames(info *compose.GraphInfo) []string {
	seen := map[string]struct{}{
		compose.START: {},
		compose.END:   {},
	}
	for name := range info.Nodes {
		seen[name] = struct{}{}
	}
	for from, tos := range info.Edges {
		seen[from] = struct{}{}
		for _, to := range tos {
			seen[to] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	names = moveNameToFront(names, compose.START)
	names = moveNameToBack(names, compose.END)
	return names
}

func moveNameToFront(values []string, target string) []string {
	for i, value := range values {
		if value == target {
			return append([]string{target}, append(values[:i], values[i+1:]...)...)
		}
	}
	return values
}

func moveNameToBack(values []string, target string) []string {
	for i, value := range values {
		if value == target {
			return append(append([]string{}, values[:i]...), append(values[i+1:], target)...)
		}
	}
	return values
}

func sortedEdgeKeys(edges map[string][]string) []string {
	keys := make([]string, 0, len(edges))
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		if key == compose.START {
			copy(keys[1:i+1], keys[0:i])
			keys[0] = compose.START
			break
		}
	}
	return keys
}

func sortedEdges(edges map[string][]string) map[string][]string {
	out := map[string][]string{}
	for _, from := range sortedEdgeKeys(edges) {
		values := append([]string(nil), edges[from]...)
		sort.Strings(values)
		out[from] = values
	}
	return out
}

func sortedGraphNames(graphs map[string]*compose.GraphInfo) []string {
	names := make([]string, 0, len(graphs))
	for name := range graphs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mermaidID(value string) string {
	switch value {
	case compose.START:
		return "START"
	case compose.END:
		return "END"
	default:
		return strings.NewReplacer("-", "_", ".", "_", ":", "_", " ", "_").Replace(value)
	}
}

func mermaidLabel(value string) string {
	switch value {
	case compose.START:
		return "START"
	case compose.END:
		return "END"
	default:
		return value
	}
}

func escapeMermaidLabel(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}
