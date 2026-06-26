package creative

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type BriefRule struct {
	Text     string `json:"text"`
	Severity string `json:"severity"`
}

type MemoryFact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MemoryRule struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
}

type MemorySource struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Note string `json:"note,omitempty"`
}

type ElementSourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Note string `json:"note,omitempty"`
}

type UpsertProjectBriefInput struct {
	WorkspaceID    pgtype.UUID
	ThreadID       pgtype.UUID
	TaskID         pgtype.UUID
	Brief          string
	Mode           string
	BriefID        string
	Title          string
	VideoType      string
	TargetAudience string
	Tone           string
	VisualStyle    string
	DurationSec    *float64
	AspectRatio    string
	Language       string
	Objective      string
	Concept        string
	Constraints    []BriefRule
	Reason         string
}

type UpdateProjectMemoryInput struct {
	WorkspaceID          pgtype.UUID
	ThreadID             pgtype.UUID
	TaskID               pgtype.UUID
	Brief                string
	Mode                 string
	CoreIntent           string
	Soul                 string
	BrandFacts           []MemoryFact
	NonNegotiables       []MemoryRule
	VisualAnchors        []MemoryFact
	Allowed              []MemoryRule
	Forbidden            []MemoryRule
	PromptInjectionHints []string
	SourceRefs           []MemorySource
	RequiresUserApproval bool
	Reason               string
}

type UpsertKeyElementsInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	Brief       string
	Mode        string
	Elements    []KeyElementInput
	Reason      string
}

type KeyElementInput struct {
	ClientKey   string
	ElementType string
	Name        string
	Description string
	SourceType  string
	SourceRefs  []ElementSourceRef
	States      []KeyElementStateInput
}

type KeyElementStateInput struct {
	ClientKey          string
	Label              string
	VisualDescription  string
	ReferenceStatus    string
	ReferenceNodeID    string
	ReferenceVersionID string
	IsDefault          bool
	StateFacts         []MemoryFact
	SourceRefs         []ElementSourceRef
}

type UpsertStoryboardInput struct {
	WorkspaceID     pgtype.UUID
	ThreadID        pgtype.UUID
	TaskID          pgtype.UUID
	Brief           string
	Mode            string
	Scope           StoryboardScope
	Scenes          []SceneInput
	Shots           []ShotInput
	ShotKeyElements []ShotKeyElementInput
	Dependencies    []ShotDependencyInput
	Reason          string
}

type StoryboardScope struct {
	Type string
	ID   string
}

type SceneInput struct {
	ClientKey   string
	SortOrder   int32
	Title       string
	Description string
	Location    string
	Mood        string
}

type ShotInput struct {
	ClientKey        string
	SceneClientKey   string
	SortOrder        int32
	Title            string
	ShotKind         string
	CreativeText     string
	NarrativePurpose string
	DurationSec      float64
	VisualIntent     string
	ActionText       string
	CameraIntent     string
	Dialogue         string
	Narration        string
	AudioPlan        AudioPlanInput
}

type AudioPlanInput struct {
	Dialogue  string `json:"dialogue,omitempty"`
	Narration string `json:"narration,omitempty"`
	SFX       string `json:"sfx,omitempty"`
	BGM       string `json:"bgm,omitempty"`
}

type ShotKeyElementInput struct {
	ShotClientKey    string
	ElementClientKey string
	StateClientKey   string
	Role             string
	Required         bool
	SortOrder        int32
}

type ShotDependencyInput struct {
	FromShotClientKey string
	ToShotClientKey   string
	DependencyType    string
	RequiredArtifact  string
	InjectionRole     string
	BlockingPhase     string
	Reason            string
}

type UpsertKeyElementsOutput struct {
	ElementsCreated int
	ElementsUpdated int
	StatesCreated   int
	StatesUpdated   int
	Elements        []db.KeyElement
	States          []db.KeyElementState
}

type UpsertStoryboardOutput struct {
	ScenesCreated       int
	ScenesUpdated       int
	ShotsCreated        int
	ShotsUpdated        int
	ShotsArchived       int
	ShotKeyElements     int
	DependenciesCreated int
	Scenes              []db.Scene
	Shots               []db.Shot
	Dependencies        []db.ShotDependency
}

type ContextPacket struct {
	Workspace       db.Workspace
	Brief           *db.CreativeBrief
	Memory          *db.ProjectMemory
	Elements        []db.KeyElement
	ElementStates   []db.KeyElementState
	Scenes          []db.Scene
	Shots           []db.Shot
	ShotKeyElements []db.ShotKeyElement
	Dependencies    []db.ShotDependency
	RenderPlans     []db.RenderPlan
}

type ReadContextInput struct {
	WorkspaceID pgtype.UUID
	ScopeType   string
	ScopeID     string
	Include     []string
	DetailLevel string
}
