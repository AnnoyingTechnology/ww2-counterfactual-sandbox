package model

type Metric struct {
	Value      float64  `json:"value"`
	Unit       string   `json:"unit"`
	HardCap    *float64 `json:"hard_cap,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	SourceNote string   `json:"source_note,omitempty"`
}

type DomainState map[string]Metric

type ActorState struct {
	Summary          string   `json:"summary,omitempty"`
	CurrentObjective string   `json:"current_objective,omitempty"`
	Notes            []string `json:"notes,omitempty"`
}

type RunMetadata struct {
	RunID               string `json:"run_id"`
	Scenario            string `json:"scenario,omitempty"`
	SourceBaseline      string `json:"source_baseline,omitempty"`
	DivergedFromHistory bool   `json:"diverged_from_history"`
	StepsExecuted       int    `json:"steps_executed"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type Snapshot struct {
	SnapshotID       string                 `json:"snapshot_id"`
	ParentSnapshotID string                 `json:"parent_snapshot_id,omitempty"`
	BranchID         string                 `json:"branch_id"`
	Date             string                 `json:"date"`
	Mode             string                 `json:"mode"`
	Actors           map[string]ActorState  `json:"actors,omitempty"`
	Domains          map[string]DomainState `json:"domains"`
	ActiveDirectives []Directive            `json:"active_directives,omitempty"`
	RecentEvents     []Event                `json:"recent_events,omitempty"`
	RunMetadata      RunMetadata            `json:"run_metadata"`
}

type Directive struct {
	ID            string  `json:"id"`
	Actor         string  `json:"actor"`
	EffectiveFrom string  `json:"effective_from"`
	EffectiveTo   string  `json:"effective_to,omitempty"`
	Scope         string  `json:"scope,omitempty"`
	Strength      string  `json:"strength"`
	Priority      float64 `json:"priority,omitempty"`
	Instruction   string  `json:"instruction"`
	Notes         string  `json:"notes,omitempty"`
	Origin        string  `json:"origin,omitempty"`
}

type Event struct {
	ID          string         `json:"id"`
	Date        string         `json:"date"`
	Actor       string         `json:"actor,omitempty"`
	Theater     string         `json:"theater,omitempty"`
	Category    string         `json:"category,omitempty"`
	Importance  float64        `json:"importance,omitempty"`
	Summary     string         `json:"summary"`
	Observables map[string]any `json:"observables,omitempty"`
}

type DirectiveResolution struct {
	Date             string   `json:"date"`
	DirectiveID      string   `json:"directive_id"`
	Status           string   `json:"status"`
	Explanation      string   `json:"explanation"`
	Blockers         []string `json:"blockers,omitempty"`
	ConcreteBlockers []string `json:"concrete_blockers,omitempty"`
}

type DerivedOrder struct {
	Actor   string `json:"actor"`
	Scope   string `json:"scope,omitempty"`
	Summary string `json:"summary"`
}

type VariableAdjustment struct {
	Domain    string  `json:"domain"`
	Variable  string  `json:"variable"`
	Operation string  `json:"operation"`
	Value     float64 `json:"value"`
	Summary   string  `json:"summary,omitempty"`
	Reason    string  `json:"reason,omitempty"`
}

type AdjudicationRecord struct {
	Date              string               `json:"date"`
	BranchID          string               `json:"branch_id"`
	RationaleSummary  string               `json:"rationale_summary"`
	Assumptions       []string             `json:"assumptions,omitempty"`
	BlockedBy         []string             `json:"blocked_by,omitempty"`
	Confidence        float64              `json:"confidence,omitempty"`
	DerivedOrders     []DerivedOrder       `json:"derived_orders,omitempty"`
	UnexpectedEffects []string             `json:"unexpected_effects,omitempty"`
	ToolCallsUsed     []string             `json:"tool_calls_used,omitempty"`
	ProposedChanges   []VariableAdjustment `json:"proposed_changes,omitempty"`
}

type MonthlyAssessment struct {
	AdjudicationRecord
	Events               []Event               `json:"events,omitempty"`
	DirectiveResolutions []DirectiveResolution `json:"directive_resolutions,omitempty"`
	SitrepHeadline       string                `json:"sitrep_headline,omitempty"`
	SitrepBody           []string              `json:"sitrep_body,omitempty"`
}

type Checkpoint struct {
	CheckpointID     string      `json:"checkpoint_id"`
	SnapshotID       string      `json:"snapshot_id"`
	BranchID         string      `json:"branch_id"`
	ParentBranchID   string      `json:"parent_branch_id,omitempty"`
	Date             string      `json:"date"`
	Mode             string      `json:"mode"`
	ActiveDirectives []Directive `json:"active_directives,omitempty"`
	Summary          string      `json:"summary,omitempty"`
	Tags             []string    `json:"tags,omitempty"`
	CreatedAt        string      `json:"created_at"`
}

type ContinuityReview struct {
	Date                      string   `json:"date"`
	BranchID                  string   `json:"branch_id"`
	Status                    string   `json:"status"`
	Warnings                  []string `json:"warnings,omitempty"`
	RecommendedSummaryRefresh []string `json:"recommended_summary_refresh,omitempty"`
	Notes                     []string `json:"notes,omitempty"`
	Confidence                float64  `json:"confidence,omitempty"`
}

type RunMeta struct {
	RunID       string   `json:"run_id"`
	Mode        string   `json:"mode"`
	Scenario    string   `json:"scenario,omitempty"`
	Description string   `json:"description,omitempty"`
	RootBranch  string   `json:"root_branch"`
	Branches    []string `json:"branches,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type BranchMeta struct {
	BranchID           string      `json:"branch_id"`
	ParentBranchID     string      `json:"parent_branch_id,omitempty"`
	ParentCheckpointID string      `json:"parent_checkpoint_id,omitempty"`
	Mode               string      `json:"mode"`
	Scenario           string      `json:"scenario,omitempty"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
	LastSnapshotID     string      `json:"last_snapshot_id,omitempty"`
	LastDate           string      `json:"last_date,omitempty"`
	ActiveDirectives   []Directive `json:"active_directives,omitempty"`
	Tags               []string    `json:"tags,omitempty"`
}

type Scenario struct {
	Name                string               `json:"name"`
	Description         string               `json:"description,omitempty"`
	BaselineSnapshot    string               `json:"baseline_snapshot,omitempty"`
	RecommendedMode     string               `json:"recommended_mode,omitempty"`
	SuggestedStartDate  string               `json:"suggested_start_date,omitempty"`
	HistoricalRationale string               `json:"historical_rationale,omitempty"`
	Directives          []Directive          `json:"directives,omitempty"`
	StateTweaks         []VariableAdjustment `json:"state_tweaks,omitempty"`
}

type ReferenceTimelineEvent struct {
	ID                    string         `json:"id"`
	DateStart             string         `json:"date_start"`
	DateEnd               string         `json:"date_end,omitempty"`
	Actors                []string       `json:"actors,omitempty"`
	Theater               string         `json:"theater,omitempty"`
	Category              string         `json:"category,omitempty"`
	Importance            float64        `json:"importance,omitempty"`
	DecisionWindow        bool           `json:"decision_window,omitempty"`
	DecisionWindowScore   float64        `json:"decision_window_score,omitempty"`
	HistoricalSummary     string         `json:"historical_summary"`
	HistoricalObservables map[string]any `json:"historical_observables,omitempty"`
	Sources               []string       `json:"sources,omitempty"`
}

type ReferenceTimelineCheckpoint struct {
	ID          string             `json:"id"`
	Date        string             `json:"date"`
	Description string             `json:"description"`
	Observables map[string]float64 `json:"observables,omitempty"`
}
