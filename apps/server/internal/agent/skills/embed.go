package skills

import (
	"embed"
	"sync"
)

//go:embed library/*/SKILL.md library/*/references/*.md
var embeddedSkills embed.FS

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
	defaultRegistryErr  error
)

func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry, defaultRegistryErr = NewRegistry(embeddedSkills)
		if defaultRegistryErr != nil {
			panic(defaultRegistryErr)
		}
	})
	return defaultRegistry
}
