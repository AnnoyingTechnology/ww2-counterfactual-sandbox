package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/llm"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/model"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/prompts"
)

type Adjudicator interface {
	AdjudicateMonth(ctx context.Context, input AdjudicationInput) (model.MonthlyAssessment, error)
	ReviewContinuity(ctx context.Context, input ContinuityInput) (model.ContinuityReview, error)
}

type AdjudicationInput struct {
	TargetDate         string
	CurrentSnapshot    model.Snapshot
	ActiveDirectives   []model.Directive
	RecentEvents       []model.Event
	HistoricalAnchors  []model.ReferenceTimelineEvent
	ContinuityWarnings []string
	Mode               string
}

type ContinuityInput struct {
	Date            string
	BranchID        string
	CurrentSnapshot model.Snapshot
	RecentRecords   []model.AdjudicationRecord
	RecentReviews   []model.ContinuityReview
}

type LLMAdjudicator struct {
	client           llm.Client
	promptPack       prompts.Pack
	detailLevel      string
	summaryLimit     int
	temperature      float64
	monthlyMaxTokens int
	reviewMaxTokens  int
}

func NewLLMAdjudicator(client llm.Client, promptPack prompts.Pack, detailLevel string, summaryLimit int, temperature float64, monthlyMaxTokens int, reviewMaxTokens int) *LLMAdjudicator {
	if monthlyMaxTokens <= 0 {
		monthlyMaxTokens = 2400
	}
	if reviewMaxTokens <= 0 {
		reviewMaxTokens = 1400
	}
	return &LLMAdjudicator{
		client:           client,
		promptPack:       promptPack,
		detailLevel:      normalizePromptDetailLevel(detailLevel),
		summaryLimit:     summaryLimit,
		temperature:      temperature,
		monthlyMaxTokens: monthlyMaxTokens,
		reviewMaxTokens:  reviewMaxTokens,
	}
}

func (a *LLMAdjudicator) AdjudicateMonth(ctx context.Context, input AdjudicationInput) (model.MonthlyAssessment, error) {
	systemPrompt, err := a.promptPack.Raw("system")
	if err != nil {
		return model.MonthlyAssessment{}, err
	}

	userPrompt, err := renderMonthlyPrompt(a.promptPack, input, a.detailLevel, a.summaryLimit)
	if err != nil {
		return model.MonthlyAssessment{}, err
	}

	var toolErr error
	if toolClient, ok := a.client.(llm.ToolCallingClient); ok {
		assessment, err := a.adjudicateMonthWithTools(ctx, toolClient, systemPrompt, userPrompt)
		if err == nil {
			assessment.Date = input.TargetDate
			assessment.BranchID = input.CurrentSnapshot.BranchID
			return assessment, nil
		}
		toolErr = err
	}

	var assessment model.MonthlyAssessment
	if err := a.client.GenerateJSON(ctx, llm.StructuredRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		SchemaName:   "monthly_assessment",
		Temperature:  a.temperature,
		MaxTokens:    a.monthlyMaxTokens,
	}, &assessment); err != nil {
		if toolErr != nil {
			return model.MonthlyAssessment{}, fmt.Errorf("tool-calling failed: %v; json fallback failed: %w", toolErr, err)
		}
		return model.MonthlyAssessment{}, err
	}

	assessment.Date = input.TargetDate
	assessment.BranchID = input.CurrentSnapshot.BranchID
	return assessment, nil
}

func (a *LLMAdjudicator) adjudicateMonthWithTools(ctx context.Context, client llm.ToolCallingClient, systemPrompt string, userPrompt string) (model.MonthlyAssessment, error) {
	toolPrompt := strings.TrimSpace(userPrompt + "\n\nUse the available tools to record the assessment. Do not return a freeform JSON object in assistant content.")
	toolCalls, err := client.GenerateToolCalls(ctx, llm.ToolRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   toolPrompt,
		Temperature:  a.temperature,
		MaxTokens:    a.monthlyMaxTokens,
		ToolChoice:   "required",
		Tools:        monthlyAssessmentTools(),
	})
	if err != nil {
		return model.MonthlyAssessment{}, err
	}

	assessment, err := monthlyAssessmentFromToolCalls(toolCalls)
	if err == nil {
		return assessment, nil
	}
	if !strings.Contains(err.Error(), "set_month_summary") {
		return model.MonthlyAssessment{}, err
	}

	summaryCalls, repairErr := client.GenerateToolCalls(ctx, llm.ToolRequest{
		SystemPrompt: systemPrompt,
		UserPrompt: strings.TrimSpace(`
The previous response recorded part of the monthly assessment through tools but omitted the required summary.

Call only the set_month_summary tool exactly once.
Do not repeat variable adjustments, events, or directive resolutions.
Provide rationale_summary, assumptions, blocked_by, confidence, unexpected_effects, sitrep_headline, and sitrep_body for the same month and branch state already under discussion.`),
		Temperature: a.temperature,
		MaxTokens:   minInt(a.monthlyMaxTokens/3, 1200),
		ToolChoice:  "required",
		Tools: []llm.ToolDefinition{
			monthlyAssessmentTools()[0],
		},
	})
	if repairErr != nil {
		return model.MonthlyAssessment{}, fmt.Errorf("%w; summary repair failed: %v", err, repairErr)
	}

	toolCalls = append(toolCalls, summaryCalls...)
	return monthlyAssessmentFromToolCalls(toolCalls)
}

func (a *LLMAdjudicator) ReviewContinuity(ctx context.Context, input ContinuityInput) (model.ContinuityReview, error) {
	projectedState, err := projectSnapshot(input.CurrentSnapshot, promptDetailSpec{
		Level:               "medium",
		MetricLimit:         24,
		MetricSummaryLimit:  maxInt(12, a.summaryLimit),
		IncludeActorNotes:   true,
		ActorNotesLimit:     2,
		IncludeSourceNotes:  false,
		RecentEventLimit:    6,
		HistoricalAnchorMax: 4,
		WarningLimit:        4,
	})
	if err != nil {
		return model.ContinuityReview{}, err
	}

	systemPrompt, err := a.promptPack.Raw("system")
	if err != nil {
		return model.ContinuityReview{}, err
	}

	userPrompt, err := a.promptPack.Render("continuity_review", map[string]any{
		"BranchID":       input.BranchID,
		"Date":           input.Date,
		"ProjectedState": projectedState,
		"RecentRecords":  mustJSON(input.RecentRecords),
		"RecentReviews":  mustJSON(input.RecentReviews),
	})
	if err != nil {
		return model.ContinuityReview{}, err
	}

	var review model.ContinuityReview
	if err := a.client.GenerateJSON(ctx, llm.StructuredRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		SchemaName:   "continuity_review",
		Temperature:  a.temperature,
		MaxTokens:    a.reviewMaxTokens,
	}, &review); err != nil {
		return model.ContinuityReview{}, err
	}

	review.Date = input.Date
	review.BranchID = input.BranchID
	return review, nil
}

type promptDetailSpec struct {
	Level               string
	MetricLimit         int
	MetricSummaryLimit  int
	IncludeActorNotes   bool
	ActorNotesLimit     int
	IncludeSourceNotes  bool
	RecentEventLimit    int
	HistoricalAnchorMax int
	WarningLimit        int
	OutputBudget        string
	DetailInstruction   string
}

func renderMonthlyPrompt(pack prompts.Pack, input AdjudicationInput, detailLevel string, summaryLimit int) (string, error) {
	data, err := buildMonthlyPromptRenderData(input, detailLevel, summaryLimit)
	if err != nil {
		return "", err
	}
	return pack.Render("monthly_adjudication", data)
}

func buildMonthlyPromptRenderData(input AdjudicationInput, detailLevel string, summaryLimit int) (map[string]any, error) {
	spec := promptDetailSpecForLevel(detailLevel, summaryLimit)
	projectedState, err := projectSnapshot(input.CurrentSnapshot, spec)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"TargetDate":             input.TargetDate,
		"CurrentDate":            input.CurrentSnapshot.Date,
		"ConstraintProfile":      input.Mode,
		"ConstraintMeaning":      modePromptInstruction(input.Mode),
		"PromptDetailLevel":      spec.Level,
		"DetailLevelInstruction": spec.DetailInstruction,
		"ProjectedState":         projectedState,
		"ActiveDirectives":       mustJSON(input.ActiveDirectives),
		"RecentEvents":           mustJSON(trimRecentEvents(input.RecentEvents, spec.RecentEventLimit)),
		"HistoricalAnchors":      mustJSON(trimHistoricalAnchors(input.HistoricalAnchors, spec)),
		"ContinuityWarnings":     mustJSON(trimStrings(input.ContinuityWarnings, spec.WarningLimit)),
		"OutputBudget":           spec.OutputBudget,
		"JSONOutputExample":      monthlyAssessmentJSONExample(),
	}, nil
}

func projectSnapshot(snapshot model.Snapshot, spec promptDetailSpec) (string, error) {
	type projectedMetric struct {
		Domain     string  `json:"domain"`
		Variable   string  `json:"variable"`
		Value      float64 `json:"value"`
		Unit       string  `json:"unit"`
		Summary    string  `json:"summary,omitempty"`
		SourceNote string  `json:"source_note,omitempty"`
	}

	type projectedActor struct {
		Summary          string   `json:"summary,omitempty"`
		CurrentObjective string   `json:"current_objective,omitempty"`
		Notes            []string `json:"notes,omitempty"`
	}

	type projection struct {
		Date    string                    `json:"date"`
		Actors  map[string]projectedActor `json:"actors,omitempty"`
		Metrics []projectedMetric         `json:"metrics"`
	}

	var metrics []projectedMetric
	for domain, variables := range snapshot.Domains {
		for name, metric := range variables {
			projected := projectedMetric{
				Domain:   domain,
				Variable: name,
				Value:    metric.Value,
				Unit:     metric.Unit,
			}
			if metric.Summary != "" && (spec.MetricSummaryLimit == 0 || len(metrics) < spec.MetricLimit || spec.MetricLimit == 0) {
				projected.Summary = metric.Summary
			}
			if spec.IncludeSourceNotes {
				projected.SourceNote = metric.SourceNote
			}
			metrics = append(metrics, projected)
		}
	}

	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].Summary == "" && metrics[j].Summary != "" {
			return false
		}
		if metrics[i].Summary != "" && metrics[j].Summary == "" {
			return true
		}
		if metrics[i].Domain == metrics[j].Domain {
			return metrics[i].Variable < metrics[j].Variable
		}
		return metrics[i].Domain < metrics[j].Domain
	})

	if spec.MetricLimit > 0 && len(metrics) > spec.MetricLimit {
		metrics = metrics[:spec.MetricLimit]
	}
	if spec.MetricSummaryLimit > 0 {
		withSummaries := 0
		for index := range metrics {
			if metrics[index].Summary == "" {
				continue
			}
			withSummaries++
			if withSummaries > spec.MetricSummaryLimit {
				metrics[index].Summary = ""
				metrics[index].SourceNote = ""
			}
		}
	}

	actors := make(map[string]projectedActor, len(snapshot.Actors))
	for name, actor := range snapshot.Actors {
		projected := projectedActor{
			Summary:          actor.Summary,
			CurrentObjective: actor.CurrentObjective,
		}
		if spec.IncludeActorNotes {
			projected.Notes = trimStrings(actor.Notes, spec.ActorNotesLimit)
		}
		actors[name] = projected
	}

	payload := projection{
		Date:    snapshot.Date,
		Actors:  actors,
		Metrics: metrics,
	}

	formatted, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

func promptDetailSpecForLevel(detailLevel string, legacySummaryLimit int) promptDetailSpec {
	level := normalizePromptDetailLevel(detailLevel)
	switch level {
	case "coarse":
		return promptDetailSpec{
			Level:               level,
			MetricLimit:         12,
			MetricSummaryLimit:  minPositiveInt(8, legacySummaryLimit),
			IncludeActorNotes:   false,
			IncludeSourceNotes:  false,
			RecentEventLimit:    3,
			HistoricalAnchorMax: 3,
			WarningLimit:        2,
			OutputBudget:        "- assumptions: 1-3\n- derived_orders: 0-2\n- proposed_changes: 1-4\n- events: 0-2\n- directive_resolutions: 0-2\n- sitrep_body: 2-4 lines",
			DetailInstruction:   "Coarse means strategic compression. Include only the highest-signal developments of the month and a very small number of state changes.",
		}
	case "fine":
		return promptDetailSpec{
			Level:               level,
			MetricLimit:         0,
			MetricSummaryLimit:  0,
			IncludeActorNotes:   true,
			ActorNotesLimit:     0,
			IncludeSourceNotes:  true,
			RecentEventLimit:    10,
			HistoricalAnchorMax: 8,
			WarningLimit:        6,
			OutputBudget:        "- assumptions: 2-6\n- derived_orders: 1-6\n- proposed_changes: 3-10\n- events: 1-6\n- directive_resolutions: 0-6\n- sitrep_body: 3-6 lines",
			DetailInstruction:   "Fine means high-detail monthly adjudication. You may describe several linked changes, but still prefer causally important items over exhaustive trivia.",
		}
	default:
		return promptDetailSpec{
			Level:               "medium",
			MetricLimit:         24,
			MetricSummaryLimit:  minPositiveInt(16, legacySummaryLimit),
			IncludeActorNotes:   true,
			ActorNotesLimit:     2,
			IncludeSourceNotes:  false,
			RecentEventLimit:    6,
			HistoricalAnchorMax: 5,
			WarningLimit:        4,
			OutputBudget:        "- assumptions: 2-4\n- derived_orders: 1-4\n- proposed_changes: 2-6\n- events: 1-4\n- directive_resolutions: 0-4\n- sitrep_body: 3-5 lines",
			DetailInstruction:   "Medium means operational monthly adjudication. Include the main causal developments, a modest set of state changes, and only the most important consequences.",
		}
	}
}

func normalizePromptDetailLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "coarse", "fine":
		return strings.ToLower(strings.TrimSpace(level))
	default:
		return "medium"
	}
}

func modePromptInstruction(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "strict":
		return "Apply high resistance to implausible departures. Institutions, ideology, and material limits should resist user steering strongly."
	case "god":
		return "Directives may override normal plausibility, but the consequences, contradictions, and costs must still be stated plainly."
	default:
		return "Stay within the actual political, ideological, and material reality of WWII. Extreme brutality and irrational escalation remain possible."
	}
}

func trimRecentEvents(events []model.Event, limit int) []model.Event {
	if limit <= 0 || len(events) <= limit {
		return events
	}
	return events[len(events)-limit:]
}

func trimHistoricalAnchors(anchors []model.ReferenceTimelineEvent, spec promptDetailSpec) []map[string]any {
	if len(anchors) == 0 {
		return nil
	}

	sorted := append([]model.ReferenceTimelineEvent{}, anchors...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Importance == sorted[j].Importance {
			return sorted[i].DateStart < sorted[j].DateStart
		}
		return sorted[i].Importance > sorted[j].Importance
	})
	if spec.HistoricalAnchorMax > 0 && len(sorted) > spec.HistoricalAnchorMax {
		sorted = sorted[:spec.HistoricalAnchorMax]
	}

	output := make([]map[string]any, 0, len(sorted))
	for _, anchor := range sorted {
		item := map[string]any{
			"id":                 anchor.ID,
			"date_start":         anchor.DateStart,
			"date_end":           anchor.DateEnd,
			"actors":             anchor.Actors,
			"theater":            anchor.Theater,
			"category":           anchor.Category,
			"importance":         anchor.Importance,
			"decision_window":    anchor.DecisionWindow,
			"historical_summary": anchor.HistoricalSummary,
		}
		if spec.IncludeSourceNotes {
			item["historical_observables"] = anchor.HistoricalObservables
			item["sources"] = anchor.Sources
		}
		output = append(output, item)
	}
	return output
}

func trimStrings(values []string, limit int) []string {
	if limit == 0 || len(values) <= limit {
		return values
	}
	return append([]string{}, values[:limit]...)
}

func monthlyAssessmentJSONExample() string {
	return `{
  "rationale_summary": "Explain the main causal developments of the month in 2-5 sentences.",
  "assumptions": [
    "Explicit assumption 1",
    "Explicit assumption 2"
  ],
  "blocked_by": [
    "Main blocker 1"
  ],
  "confidence": 0.72,
  "derived_orders": [
    {
      "actor": "germany",
      "scope": "eastern_front",
      "summary": "Concrete operational order derived from higher-level intent."
    }
  ],
  "unexpected_effects": [
    "Important side effect of this month's developments."
  ],
  "proposed_changes": [
    {
      "domain": "logistics_fronts",
      "variable": "supply_status_east",
      "operation": "delta",
      "value": -0.04,
      "summary": "Why this variable changes.",
      "reason": "Short causal reference."
    }
  ],
  "events": [
    {
      "id": "event_1941_07_example",
      "date": "1941-07",
      "actor": "germany",
      "theater": "eastern_front",
      "category": "military",
      "importance": 0.67,
      "summary": "One important event from the month.",
      "observables": {
        "front_shift_km": 180
      }
    }
  ],
  "directive_resolutions": [
    {
      "date": "1941-07",
      "directive_id": "dir_example",
      "status": "partially_followed",
      "explanation": "How the directive actually resolved.",
      "blockers": [
        "High-level blocker"
      ],
      "concrete_blockers": [
        "Specific threshold-style blocker with context."
      ]
    }
  ],
  "sitrep_headline": "Short headline",
  "sitrep_body": [
    "Short sitrep line 1",
    "Short sitrep line 2"
  ]
}`
}

func mustJSON(value any) string {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, strings.ReplaceAll(err.Error(), `"`, `'`))
	}
	return string(payload)
}

func monthlyAssessmentTools() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        "set_month_summary",
			Description: "Write the once-per-month strategic summary, confidence, blockers, and sitrep. Call this exactly once.",
			Parameters: objectSchema(
				map[string]any{
					"rationale_summary":  stringSchema("Concise explanation of why the branch moved the way it did this month."),
					"assumptions":        stringArraySchema("Short explicit assumptions behind the assessment."),
					"blocked_by":         stringArraySchema("High-level blockers that constrained branch behavior this month."),
					"confidence":         numberSchema("0 to 1 confidence in the monthly assessment."),
					"unexpected_effects": stringArraySchema("Secondary effects that were not the main goal of actor decisions."),
					"sitrep_headline":    stringSchema("Short headline for the monthly sitrep."),
					"sitrep_body":        stringArraySchema("A few concise lines for the sitrep body."),
				},
				[]string{"rationale_summary"},
			),
		},
		{
			Name:        "add_derived_order",
			Description: "Record one concrete derived order implied by a directive or actor intent.",
			Parameters: objectSchema(
				map[string]any{
					"actor":   stringSchema("Actor issuing or receiving the order."),
					"scope":   stringSchema("Operational or political scope."),
					"summary": stringSchema("Concrete order derived from higher-level intent."),
				},
				[]string{"actor", "summary"},
			),
		},
		{
			Name:        "add_variable_adjustment",
			Description: "Propose one metric or state adjustment for deterministic application.",
			Parameters: objectSchema(
				map[string]any{
					"domain":    stringSchema("Top-level state domain."),
					"variable":  stringSchema("Variable name inside the domain."),
					"operation": stringSchema("Adjustment type such as delta, set, floor, or cap."),
					"value":     numberSchema("Numeric adjustment value."),
					"summary":   stringSchema("Short explanation of the adjustment."),
					"reason":    stringSchema("Directive id, event id, or short causal reason."),
				},
				[]string{"domain", "variable", "operation", "value"},
			),
		},
		{
			Name:        "add_event",
			Description: "Record one noteworthy event produced by this month's adjudication.",
			Parameters: objectSchema(
				map[string]any{
					"id":          stringSchema("Stable event id."),
					"date":        stringSchema("Month or date of the event."),
					"actor":       stringSchema("Primary actor."),
					"theater":     stringSchema("Theater or arena."),
					"category":    stringSchema("Event category."),
					"importance":  numberSchema("0 to 1 importance score."),
					"summary":     stringSchema("Concise event summary."),
					"observables": map[string]any{"type": "object"},
				},
				[]string{"summary"},
			),
		},
		{
			Name:        "resolve_directive",
			Description: "Record how one active directive resolved this month, including concrete blockers if relevant.",
			Parameters: objectSchema(
				map[string]any{
					"date":              stringSchema("Month of the resolution."),
					"directive_id":      stringSchema("Directive id being resolved."),
					"status":            stringSchema("Resolution status such as followed, partially_followed, blocked."),
					"explanation":       stringSchema("Short explanation of the resolution."),
					"blockers":          stringArraySchema("High-level blocker labels."),
					"concrete_blockers": stringArraySchema("Specific threshold-style blockers with variables or causal specifics."),
				},
				[]string{"directive_id", "status", "explanation"},
			),
		},
	}
}

func monthlyAssessmentFromToolCalls(calls []llm.ToolCall) (model.MonthlyAssessment, error) {
	type summaryArgs struct {
		RationaleSummary  string   `json:"rationale_summary"`
		Assumptions       []string `json:"assumptions,omitempty"`
		BlockedBy         []string `json:"blocked_by,omitempty"`
		Confidence        float64  `json:"confidence,omitempty"`
		UnexpectedEffects []string `json:"unexpected_effects,omitempty"`
		SitrepHeadline    string   `json:"sitrep_headline,omitempty"`
		SitrepBody        []string `json:"sitrep_body,omitempty"`
	}

	assessment := model.MonthlyAssessment{}
	sawSummary := false
	for _, call := range calls {
		switch call.Name {
		case "set_month_summary":
			var args summaryArgs
			if err := json.Unmarshal(call.Arguments, &args); err != nil {
				return model.MonthlyAssessment{}, fmt.Errorf("invalid set_month_summary arguments: %w", err)
			}
			assessment.RationaleSummary = args.RationaleSummary
			assessment.Assumptions = args.Assumptions
			assessment.BlockedBy = args.BlockedBy
			assessment.Confidence = args.Confidence
			assessment.UnexpectedEffects = args.UnexpectedEffects
			assessment.SitrepHeadline = args.SitrepHeadline
			assessment.SitrepBody = args.SitrepBody
			sawSummary = true
		case "add_derived_order":
			var order model.DerivedOrder
			if err := json.Unmarshal(call.Arguments, &order); err != nil {
				return model.MonthlyAssessment{}, fmt.Errorf("invalid add_derived_order arguments: %w", err)
			}
			assessment.DerivedOrders = append(assessment.DerivedOrders, order)
		case "add_variable_adjustment":
			var adjustment model.VariableAdjustment
			if err := json.Unmarshal(call.Arguments, &adjustment); err != nil {
				return model.MonthlyAssessment{}, fmt.Errorf("invalid add_variable_adjustment arguments: %w", err)
			}
			assessment.ProposedChanges = append(assessment.ProposedChanges, adjustment)
		case "add_event":
			var event model.Event
			if err := json.Unmarshal(call.Arguments, &event); err != nil {
				return model.MonthlyAssessment{}, fmt.Errorf("invalid add_event arguments: %w", err)
			}
			assessment.Events = append(assessment.Events, event)
		case "resolve_directive":
			var resolution model.DirectiveResolution
			if err := json.Unmarshal(call.Arguments, &resolution); err != nil {
				return model.MonthlyAssessment{}, fmt.Errorf("invalid resolve_directive arguments: %w", err)
			}
			assessment.DirectiveResolutions = append(assessment.DirectiveResolutions, resolution)
		default:
			return model.MonthlyAssessment{}, fmt.Errorf("unknown tool call %q", call.Name)
		}

		assessment.ToolCallsUsed = append(assessment.ToolCallsUsed, "llm:"+call.Name)
	}

	if !sawSummary || strings.TrimSpace(assessment.RationaleSummary) == "" {
		return model.MonthlyAssessment{}, fmt.Errorf("tool-calling response did not include set_month_summary with rationale_summary")
	}

	return assessment, nil
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": "string",
		},
	}
}

func numberSchema(description string) map[string]any {
	return map[string]any{
		"type":        "number",
		"description": description,
	}
}

func minInt(values ...int) int {
	best := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if best == 0 || value < best {
			best = value
		}
	}
	return best
}

func minPositiveInt(values ...int) int {
	best := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if best == 0 || value < best {
			best = value
		}
	}
	return best
}

func maxInt(values ...int) int {
	best := 0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}
