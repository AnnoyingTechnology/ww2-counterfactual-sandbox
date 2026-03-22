package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/model"
)

type MockAdjudicator struct{}

func NewMockAdjudicator() *MockAdjudicator {
	return &MockAdjudicator{}
}

func (m *MockAdjudicator) AdjudicateMonth(_ context.Context, input AdjudicationInput) (model.MonthlyAssessment, error) {
	assessment := model.MonthlyAssessment{
		AdjudicationRecord: model.AdjudicationRecord{
			Date:              input.TargetDate,
			BranchID:          input.CurrentSnapshot.BranchID,
			Confidence:        0.64,
			ToolCallsUsed:     []string{"mock_strategic_heuristics"},
			Assumptions:       []string{"The sandbox is using the built-in mock adjudicator rather than a real LLM.", "Monthly state changes remain intentionally small in the initial skeleton."},
			BlockedBy:         []string{},
			UnexpectedEffects: []string{},
		},
		SitrepHeadline: fmt.Sprintf("%s strategic overview", input.TargetDate),
	}

	assessment.RationaleSummary = "The branch advances through a single monthly strategic pass that balances operational tempo, political friction, resource drawdown, and any active directives."
	assessment.ProposedChanges = append(assessment.ProposedChanges,
		model.VariableAdjustment{
			Domain:    "raw_materials_energy",
			Variable:  "oil_stockpile",
			Operation: "delta",
			Value:     -2.5,
			Summary:   "Routine operational burn and transport consumption continue to draw down reserves.",
			Reason:    "baseline monthly burn",
		},
		model.VariableAdjustment{
			Domain:    "logistics_fronts",
			Variable:  "supply_status_east",
			Operation: "delta",
			Value:     -0.02,
			Summary:   "Distance, rail friction, and maintenance strain continue to erode eastern supply reliability.",
			Reason:    "baseline logistics friction",
		},
		model.VariableAdjustment{
			Domain:    "diplomacy_external_relations",
			Variable:  "negotiated_peace_feasibility",
			Operation: "delta",
			Value:     -0.01,
			Summary:   "Absent a major policy shock, the war remains politically unreconciled.",
			Reason:    "baseline diplomatic hardening",
		},
	)

	if input.TargetDate <= "1941-09" {
		assessment.ProposedChanges = append(assessment.ProposedChanges, model.VariableAdjustment{
			Domain:    "logistics_fronts",
			Variable:  "front_position_east",
			Operation: "delta",
			Value:     0.04,
			Summary:   "Operational momentum still produces local gains, though supply strain grows in parallel.",
			Reason:    "summer campaign pressure",
		})
	}

	for _, directive := range input.ActiveDirectives {
		lower := strings.ToLower(strings.Join([]string{directive.Scope, directive.Instruction, directive.Notes}, " "))
		resolution := model.DirectiveResolution{
			Date:        input.TargetDate,
			DirectiveID: directive.ID,
			Status:      "followed",
		}

		derived := model.DerivedOrder{
			Actor:   directive.Actor,
			Scope:   directive.Scope,
			Summary: directive.Instruction,
		}

		switch {
		case hasAny(lower, "withdraw", "avoid", "preserve"):
			leadership := metricValue(input.CurrentSnapshot, "politics_friction", "leadership_interference")
			if leadership >= 75 && directive.Strength != "god" {
				resolution.Status = "partially_followed"
				resolution.Explanation = "Withdrawal or force-preservation intent met leadership resistance before it could fully translate into operational orders."
				resolution.Blockers = []string{"political resistance", "timing"}
				resolution.ConcreteBlockers = []string{
					fmt.Sprintf("leadership_interference=%.1f keeps withdrawal authority below the threshold implied by the directive", leadership),
					"front supply is already stressed, so disengagement remains incomplete even where ordered",
				}
				assessment.BlockedBy = append(assessment.BlockedBy, "leadership interference delayed or narrowed a force-preservation directive")
			} else {
				resolution.Explanation = "The branch accepted a force-preservation posture and traded local prestige for preserving combat power."
			}

			derived.Summary = "Convert the high-level preservation directive into concrete withdrawal authority, rear-area rail prioritization, and flank security orders."
			assessment.ProposedChanges = append(assessment.ProposedChanges,
				model.VariableAdjustment{
					Domain:    "logistics_fronts",
					Variable:  "supply_status_east",
					Operation: "delta",
					Value:     0.05,
					Summary:   "Force-preservation orders reduce wasteful exposure and improve recoverable supply throughput.",
					Reason:    directive.ID,
				},
				model.VariableAdjustment{
					Domain:    "logistics_fronts",
					Variable:  "front_position_east",
					Operation: "delta",
					Value:     -0.03,
					Summary:   "Operational space is traded away to preserve formations and transport capacity.",
					Reason:    directive.ID,
				},
				model.VariableAdjustment{
					Domain:    "politics_friction",
					Variable:  "policy_flexibility",
					Operation: "delta",
					Value:     3,
					Summary:   "The regime demonstrates some willingness to reverse prestige choices before total disaster.",
					Reason:    directive.ID,
				},
			)
		case hasAny(lower, "fuel", "repair", "production", "truck"):
			resolution.Explanation = "Resource allocation shifted toward sustainment, repair, and transport capacity."
			derived.Summary = "Prioritize fuel, repair, and transport programs over prestige allocation."
			assessment.ProposedChanges = append(assessment.ProposedChanges,
				model.VariableAdjustment{
					Domain:    "raw_materials_energy",
					Variable:  "synthetic_fuel_output",
					Operation: "delta",
					Value:     1.2,
					Summary:   "Administrative priority and better repair discipline nudge monthly fuel output upward.",
					Reason:    directive.ID,
				},
				model.VariableAdjustment{
					Domain:    "strategic_bombing_repair",
					Variable:  "repair_capacity_industry",
					Operation: "delta",
					Value:     0.03,
					Summary:   "Repair assets are prioritized and localized bottlenecks ease slightly.",
					Reason:    directive.ID,
				},
			)
		case hasAny(lower, "peace", "truce", "ceasefire", "settlement"):
			genocide := metricValue(input.CurrentSnapshot, "atrocity_repression", "genocidal_policy_intensity")
			rigidity := metricValue(input.CurrentSnapshot, "politics_friction", "ideological_rigidity")
			if (genocide >= 0.55 || rigidity >= 85) && directive.Strength != "god" {
				resolution.Status = "blocked"
				resolution.Explanation = "Peace feelers could not gain traction because policy reality and ideological commitments remain too radicalized."
				resolution.Blockers = []string{"ideological rigidity", "policy mismatch"}
				resolution.ConcreteBlockers = []string{
					fmt.Sprintf("genocidal_policy_intensity=%.2f keeps outside actors from treating peace feelers as credible", genocide),
					fmt.Sprintf("ideological_rigidity=%.1f prevents the regime from making concessions implied by the directive", rigidity),
				}
				assessment.BlockedBy = append(assessment.BlockedBy, "peace initiative blocked by ideological commitments and credibility collapse")
			} else {
				resolution.Explanation = "Diplomatic probing becomes more active and marginally less self-defeating."
				assessment.ProposedChanges = append(assessment.ProposedChanges, model.VariableAdjustment{
					Domain:    "diplomacy_external_relations",
					Variable:  "negotiated_peace_feasibility",
					Operation: "delta",
					Value:     0.08,
					Summary:   "The branch creates more room for indirect contacts and exploratory bargaining.",
					Reason:    directive.ID,
				})
			}
		case hasAny(lower, "exterminate", "annihilate", "deport", "purge", "liquidate", "terror"):
			resolution.Explanation = "The directive radicalizes coercive policy and intensifies occupation and repression burdens."
			derived.Summary = "Escalate coercive and genocidal policy at the state level; administrative pressure rises across transport, secrecy, and occupation systems."
			assessment.ProposedChanges = append(assessment.ProposedChanges,
				model.VariableAdjustment{
					Domain:    "atrocity_repression",
					Variable:  "genocidal_policy_intensity",
					Operation: "delta",
					Value:     0.08,
					Summary:   "Policy escalates toward more systematic mass murder and coercive control.",
					Reason:    directive.ID,
				},
				model.VariableAdjustment{
					Domain:    "atrocity_repression",
					Variable:  "occupation_brutality",
					Operation: "delta",
					Value:     0.06,
					Summary:   "Occupation violence and punitive behavior intensify.",
					Reason:    directive.ID,
				},
				model.VariableAdjustment{
					Domain:    "logistics_fronts",
					Variable:  "partisan_pressure",
					Operation: "delta",
					Value:     0.05,
					Summary:   "Harsher occupation and repression feed resistance and rear-area insecurity.",
					Reason:    directive.ID,
				},
				model.VariableAdjustment{
					Domain:    "diplomacy_external_relations",
					Variable:  "negotiated_peace_feasibility",
					Operation: "delta",
					Value:     -0.05,
					Summary:   "Escalating atrocity further destroys peace credibility and hardens enemy resolve.",
					Reason:    directive.ID,
				},
			)
			assessment.UnexpectedEffects = append(assessment.UnexpectedEffects, "Escalated repression increases administrative drag, diplomatic isolation, and rear-area violence.")
		default:
			resolution.Explanation = "The directive shaped the branch, but without a specialized rule it only biases the monthly narrative rather than moving a dedicated subsystem."
		}

		assessment.DerivedOrders = append(assessment.DerivedOrders, derived)
		assessment.DirectiveResolutions = append(assessment.DirectiveResolutions, resolution)
	}

	if len(assessment.DirectiveResolutions) == 0 {
		assessment.SitrepBody = append(assessment.SitrepBody, "No active directives altered the branch this month, so baseline strategic drift dominated.")
	} else {
		assessment.SitrepBody = append(assessment.SitrepBody, fmt.Sprintf("%d directive(s) were evaluated during the month.", len(assessment.DirectiveResolutions)))
	}

	assessment.Events = append(assessment.Events, model.Event{
		ID:         fmt.Sprintf("monthly_assessment_%s", strings.ReplaceAll(input.TargetDate, "-", "")),
		Date:       input.TargetDate,
		Category:   "strategic",
		Importance: 0.45,
		Summary:    "Monthly strategic adjudication completed and proposed state changes were emitted for deterministic application.",
	})

	return assessment, nil
}

func (m *MockAdjudicator) ReviewContinuity(_ context.Context, input ContinuityInput) (model.ContinuityReview, error) {
	review := model.ContinuityReview{
		Date:       input.Date,
		BranchID:   input.BranchID,
		Status:     "clean",
		Confidence: 0.67,
	}

	oil := metricValue(input.CurrentSnapshot, "raw_materials_energy", "oil_stockpile")
	supply := metricValue(input.CurrentSnapshot, "logistics_fronts", "supply_status_east")
	genocide := metricValue(input.CurrentSnapshot, "atrocity_repression", "genocidal_policy_intensity")
	peace := metricValue(input.CurrentSnapshot, "diplomacy_external_relations", "negotiated_peace_feasibility")

	if oil < 20 && supply > 0.7 {
		review.Status = "needs_attention"
		review.Warnings = append(review.Warnings, "Eastern supply still reads strong despite a dangerously thin oil reserve.")
		review.RecommendedSummaryRefresh = append(review.RecommendedSummaryRefresh, "raw_materials_energy.oil_stockpile", "logistics_fronts.supply_status_east")
	}
	if genocide > 0.7 && peace > 0.3 {
		review.Status = "needs_attention"
		review.Warnings = append(review.Warnings, "Peace feasibility remains too optimistic for a branch with heavily escalated genocidal policy.")
		review.RecommendedSummaryRefresh = append(review.RecommendedSummaryRefresh, "diplomacy_external_relations.negotiated_peace_feasibility", "atrocity_repression.genocidal_policy_intensity")
	}
	if len(review.Warnings) == 0 {
		review.Notes = append(review.Notes, "No obvious continuity contradiction was detected by the mock reviewer.")
	}

	return review, nil
}

func hasAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
