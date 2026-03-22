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
	client       llm.Client
	promptPack   prompts.Pack
	summaryLimit int
}

func NewLLMAdjudicator(client llm.Client, promptPack prompts.Pack, summaryLimit int) *LLMAdjudicator {
	return &LLMAdjudicator{
		client:       client,
		promptPack:   promptPack,
		summaryLimit: summaryLimit,
	}
}

func (a *LLMAdjudicator) AdjudicateMonth(ctx context.Context, input AdjudicationInput) (model.MonthlyAssessment, error) {
	projectedState, err := projectSnapshot(input.CurrentSnapshot, a.summaryLimit)
	if err != nil {
		return model.MonthlyAssessment{}, err
	}

	systemPrompt, err := a.promptPack.Raw("system")
	if err != nil {
		return model.MonthlyAssessment{}, err
	}

	userPrompt, err := a.promptPack.Render("monthly_adjudication", map[string]any{
		"TargetDate":         input.TargetDate,
		"Mode":               input.Mode,
		"BranchID":           input.CurrentSnapshot.BranchID,
		"CurrentDate":        input.CurrentSnapshot.Date,
		"ProjectedState":     projectedState,
		"ActiveDirectives":   mustJSON(input.ActiveDirectives),
		"RecentEvents":       mustJSON(input.RecentEvents),
		"HistoricalAnchors":  mustJSON(input.HistoricalAnchors),
		"ContinuityWarnings": mustJSON(input.ContinuityWarnings),
	})
	if err != nil {
		return model.MonthlyAssessment{}, err
	}

	var assessment model.MonthlyAssessment
	if err := a.client.GenerateJSON(ctx, llm.StructuredRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		SchemaName:   "monthly_assessment",
		Temperature:  0.3,
		MaxTokens:    2400,
	}, &assessment); err != nil {
		return model.MonthlyAssessment{}, err
	}

	assessment.Date = input.TargetDate
	assessment.BranchID = input.CurrentSnapshot.BranchID
	return assessment, nil
}

func (a *LLMAdjudicator) ReviewContinuity(ctx context.Context, input ContinuityInput) (model.ContinuityReview, error) {
	projectedState, err := projectSnapshot(input.CurrentSnapshot, a.summaryLimit)
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
		Temperature:  0.2,
		MaxTokens:    1400,
	}, &review); err != nil {
		return model.ContinuityReview{}, err
	}

	review.Date = input.Date
	review.BranchID = input.BranchID
	return review, nil
}

func projectSnapshot(snapshot model.Snapshot, summaryLimit int) (string, error) {
	type projectedMetric struct {
		Domain     string  `json:"domain"`
		Variable   string  `json:"variable"`
		Value      float64 `json:"value"`
		Unit       string  `json:"unit"`
		Summary    string  `json:"summary,omitempty"`
		SourceNote string  `json:"source_note,omitempty"`
	}

	type projection struct {
		Date             string                      `json:"date"`
		Mode             string                      `json:"mode"`
		Actors           map[string]model.ActorState `json:"actors,omitempty"`
		Metrics          []projectedMetric           `json:"metrics"`
		ActiveDirectives []model.Directive           `json:"active_directives,omitempty"`
	}

	var metrics []projectedMetric
	for domain, variables := range snapshot.Domains {
		for name, metric := range variables {
			metrics = append(metrics, projectedMetric{
				Domain:     domain,
				Variable:   name,
				Value:      metric.Value,
				Unit:       metric.Unit,
				Summary:    metric.Summary,
				SourceNote: metric.SourceNote,
			})
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

	if summaryLimit > 0 {
		withSummaries := 0
		for index := range metrics {
			if metrics[index].Summary == "" {
				continue
			}
			withSummaries++
			if withSummaries > summaryLimit {
				metrics[index].Summary = ""
			}
		}
	}

	payload := projection{
		Date:             snapshot.Date,
		Mode:             snapshot.Mode,
		Actors:           snapshot.Actors,
		Metrics:          metrics,
		ActiveDirectives: snapshot.ActiveDirectives,
	}

	formatted, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

func mustJSON(value any) string {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, strings.ReplaceAll(err.Error(), `"`, `'`))
	}
	return string(payload)
}
