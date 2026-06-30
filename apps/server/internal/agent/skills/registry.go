package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	ErrSkillNotFound   = errors.New("agent skill not found")
	ErrSkillRoleDenied = errors.New("agent skill role denied")
	ErrSkillTaskDenied = errors.New("agent skill task type denied")
	ErrSkillResource   = errors.New("agent skill resource denied")
)

type Registry struct {
	fsys      fs.FS
	byName    map[string]LoadedSkill
	skillDirs map[string]string
	mu        sync.Mutex
	usage     map[string]int
}

func NewRegistry(fsys fs.FS) (*Registry, error) {
	registry := &Registry{
		fsys:      fsys,
		byName:    map[string]LoadedSkill{},
		skillDirs: map[string]string{},
		usage:     map[string]int{},
	}
	if fsys == nil {
		return registry, nil
	}
	err := fs.WalkDir(fsys, "library", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Base(path) != "SKILL.md" {
			return nil
		}
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		loaded, err := parseSkillFile(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if _, exists := registry.byName[loaded.Name]; exists {
			return fmt.Errorf("%s: duplicate skill name %q", path, loaded.Name)
		}
		registry.byName[loaded.Name] = loaded
		registry.skillDirs[loaded.Name] = filepath.ToSlash(filepath.Dir(path))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return registry, nil
}

func (r *Registry) SkillsForRole(role Role) []SkillMetadata {
	if r == nil {
		return nil
	}
	out := []SkillMetadata{}
	for _, skill := range r.byName {
		if hasRole(skill.RoleScope, role) {
			out = append(out, skill.SkillMetadata)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) Load(name string, role Role, taskType string) (LoadedSkill, error) {
	return r.load(name, role, taskType, true)
}

func (r *Registry) LoadResource(name string, role Role, taskType string, resourcePath string) (LoadedSkillResource, error) {
	skill, err := r.load(name, role, taskType, false)
	if err != nil {
		return LoadedSkillResource{}, err
	}
	cleanResource, err := cleanResourcePath(resourcePath)
	if err != nil {
		return LoadedSkillResource{}, err
	}
	dir := r.skillDirs[skill.Name]
	if strings.TrimSpace(dir) == "" || r.fsys == nil {
		return LoadedSkillResource{}, ErrSkillResource
	}
	fullPath := path.Join(dir, cleanResource)
	raw, err := fs.ReadFile(r.fsys, fullPath)
	if err != nil {
		return LoadedSkillResource{}, ErrSkillResource
	}
	hash := sha256.Sum256(raw)
	r.incrementUsage("resource", skill.Name+"/"+cleanResource)
	return LoadedSkillResource{
		Name:         skill.Name,
		ResourcePath: cleanResource,
		Version:      skill.Version,
		Content:      strings.TrimSpace(string(raw)),
		SourceHash:   "sha256:" + hex.EncodeToString(hash[:]),
	}, nil
}

func (r *Registry) UsageStats() []UsageStat {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]UsageStat, 0, len(r.usage))
	for key, count := range r.usage {
		kind, name, _ := strings.Cut(key, ":")
		out = append(out, UsageStat{Kind: kind, Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func (r *Registry) load(name string, role Role, taskType string, countUsage bool) (LoadedSkill, error) {
	if r == nil {
		return LoadedSkill{}, ErrSkillNotFound
	}
	name = strings.TrimSpace(name)
	skill, ok := r.byName[name]
	if !ok {
		return LoadedSkill{}, ErrSkillNotFound
	}
	if !hasRole(skill.RoleScope, role) {
		return LoadedSkill{}, ErrSkillRoleDenied
	}
	if len(skill.TaskTypes) > 0 && strings.TrimSpace(taskType) != "" && !hasString(skill.TaskTypes, taskType) {
		return LoadedSkill{}, ErrSkillTaskDenied
	}
	if countUsage {
		r.incrementUsage("skill", skill.Name)
	}
	return skill, nil
}

func (r *Registry) incrementUsage(kind string, name string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usage == nil {
		r.usage = map[string]int{}
	}
	r.usage[kind+":"+name]++
}

func parseSkillFile(raw []byte) (LoadedSkill, error) {
	text := string(raw)
	if !strings.HasPrefix(text, "---\n") {
		return LoadedSkill{}, errors.New("missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(text, "---\n")
	parts := strings.SplitN(rest, "\n---\n", 2)
	if len(parts) != 2 {
		return LoadedSkill{}, errors.New("unterminated YAML frontmatter")
	}
	var meta SkillMetadata
	if err := yaml.Unmarshal([]byte(parts[0]), &meta); err != nil {
		return LoadedSkill{}, err
	}
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Description = strings.TrimSpace(meta.Description)
	meta.Version = strings.TrimSpace(meta.Version)
	if meta.Name == "" {
		return LoadedSkill{}, errors.New("name is required")
	}
	if meta.Description == "" {
		return LoadedSkill{}, errors.New("description is required")
	}
	if meta.Version == "" {
		return LoadedSkill{}, errors.New("version is required")
	}
	if len(meta.RoleScope) == 0 {
		return LoadedSkill{}, errors.New("role_scope is required")
	}
	content := strings.TrimSpace(parts[1])
	hash := sha256.Sum256(raw)
	return LoadedSkill{
		SkillMetadata: meta,
		Content:       content,
		SourceHash:    "sha256:" + hex.EncodeToString(hash[:]),
	}, nil
}

func hasRole(roles []Role, role Role) bool {
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func hasString(values []string, value string) bool {
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func cleanResourcePath(value string) (string, error) {
	value = strings.TrimSpace(filepath.ToSlash(value))
	if value == "" || strings.HasPrefix(value, "/") {
		return "", ErrSkillResource
	}
	cleaned := path.Clean(value)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." || strings.Contains(cleaned, "/../") {
		return "", ErrSkillResource
	}
	if filepath.Ext(cleaned) != ".md" {
		return "", ErrSkillResource
	}
	return cleaned, nil
}
