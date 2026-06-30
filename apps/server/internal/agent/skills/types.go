package skills

type Role string

const (
	RoleProducer  Role = "producer"
	RoleCraftsman Role = "craftsman"
	RoleReviewer  Role = "reviewer"
	RoleComposer  Role = "composer"
)

type SkillMetadata struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	RoleScope   []Role         `yaml:"role_scope" json:"role_scope"`
	TaskTypes   []string       `yaml:"task_types" json:"task_types"`
	Domain      []string       `yaml:"domain" json:"domain"`
	Tools       []string       `yaml:"tools" json:"tools"`
	Source      map[string]any `yaml:"source" json:"source,omitempty"`
	Version     string         `yaml:"version" json:"version"`
}

type LoadedSkill struct {
	SkillMetadata
	Content    string `json:"content"`
	SourceHash string `json:"source_hash"`
}

type LoadedSkillResource struct {
	Name         string `json:"name"`
	ResourcePath string `json:"resource_path"`
	Version      string `json:"version"`
	Content      string `json:"content"`
	SourceHash   string `json:"source_hash"`
}

type UsageStat struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}
