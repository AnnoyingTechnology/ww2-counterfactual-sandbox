package engine

import (
	"encoding/json"
	"testing"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/llm"
)

func TestMonthlyAssessmentFromToolCalls(t *testing.T) {
	calls := []llm.ToolCall{
		{
			Name: "set_month_summary",
			Arguments: mustRawJSON(t, map[string]any{
				"rationale_summary": "Operational momentum persists but logistics drag rises.",
				"assumptions":       []string{"Rail conversion remains the pacing bottleneck."},
				"blocked_by":        []string{"fuel strain"},
				"confidence":        0.68,
				"unexpected_effects": []string{
					"Rear-area partisan activity rises.",
				},
				"sitrep_headline": "Momentum with rising drag",
				"sitrep_body":     []string{"Advance continues unevenly."},
			}),
		},
		{
			Name: "add_variable_adjustment",
			Arguments: mustRawJSON(t, map[string]any{
				"domain":    "logistics_fronts",
				"variable":  "supply_status_east",
				"operation": "delta",
				"value":     -0.03,
				"summary":   "Longer lines worsen supply reliability.",
			}),
		},
		{
			Name: "add_event",
			Arguments: mustRawJSON(t, map[string]any{
				"id":       "smolensk_pressure",
				"date":     "1941-07",
				"category": "military",
				"summary":  "The central axis remains under heavy operational pressure.",
			}),
		},
		{
			Name: "resolve_directive",
			Arguments: mustRawJSON(t, map[string]any{
				"directive_id": "dir_01",
				"status":       "partially_followed",
				"explanation":  "Intent was accepted but transport capacity stayed constrained.",
			}),
		},
	}

	assessment, err := monthlyAssessmentFromToolCalls(calls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if assessment.RationaleSummary == "" {
		t.Fatal("expected rationale summary to be populated")
	}
	if len(assessment.ProposedChanges) != 1 {
		t.Fatalf("expected 1 proposed change, got %d", len(assessment.ProposedChanges))
	}
	if len(assessment.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(assessment.Events))
	}
	if len(assessment.DirectiveResolutions) != 1 {
		t.Fatalf("expected 1 directive resolution, got %d", len(assessment.DirectiveResolutions))
	}
	if len(assessment.ToolCallsUsed) != len(calls) {
		t.Fatalf("expected %d recorded tool calls, got %d", len(calls), len(assessment.ToolCallsUsed))
	}
}

func TestMonthlyAssessmentFromToolCallsRequiresSummary(t *testing.T) {
	_, err := monthlyAssessmentFromToolCalls([]llm.ToolCall{
		{
			Name: "add_variable_adjustment",
			Arguments: mustRawJSON(t, map[string]any{
				"domain":    "raw_materials_energy",
				"variable":  "oil_stockpile",
				"operation": "delta",
				"value":     -2.5,
			}),
		},
	})
	if err == nil {
		t.Fatal("expected error when summary tool call is missing")
	}
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return json.RawMessage(payload)
}
