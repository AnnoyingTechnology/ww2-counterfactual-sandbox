package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/model"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/storage"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/timeutil"
)

func filterActiveDirectives(directives []model.Directive, date string) []model.Directive {
	var active []model.Directive
	for _, directive := range directives {
		if timeutil.InRange(date, directive.EffectiveFrom, directive.EffectiveTo) {
			active = append(active, directive)
		}
	}
	return active
}

func applyAdjustments(snapshot *model.Snapshot, adjustments []model.VariableAdjustment) {
	for _, adjustment := range adjustments {
		if snapshot.Domains == nil {
			snapshot.Domains = make(map[string]model.DomainState)
		}

		domain := snapshot.Domains[adjustment.Domain]
		if domain == nil {
			domain = model.DomainState{}
		}

		metric, ok := domain[adjustment.Variable]
		if !ok {
			metric = inferredMetric(adjustment.Variable)
		}

		switch adjustment.Operation {
		case "set":
			metric.Value = adjustment.Value
		default:
			metric.Value += adjustment.Value
		}

		if adjustment.Summary != "" {
			metric.Summary = adjustment.Summary
		}
		if adjustment.Reason != "" {
			metric.SourceNote = adjustment.Reason
		}
		if metric.HardCap != nil && metric.Value > *metric.HardCap {
			metric.Value = *metric.HardCap
		}
		if metric.Value < 0 {
			metric.Value = 0
		}

		domain[adjustment.Variable] = metric
		snapshot.Domains[adjustment.Domain] = domain
	}
}

func computeFuelBalance(snapshot *model.Snapshot, date string) []model.Event {
	oil := metricValue(*snapshot, "raw_materials_energy", "oil_stockpile")
	synthetic := metricValue(*snapshot, "raw_materials_energy", "synthetic_fuel_output")
	damage := metricValue(*snapshot, "strategic_bombing_repair", "bombing_damage_fuel")

	delta := synthetic/6.0 - 3.0 - damage*2.0
	oilMetric := ensureMetric(snapshot, "raw_materials_energy", "oil_stockpile", model.Metric{
		Value:   0,
		Unit:    "stockpile_index",
		HardCap: floatPtr(100),
	})
	oilMetric.Value = oil + delta
	if oilMetric.Value < 0 {
		oilMetric.Value = 0
	}
	oilMetric.Summary = "Oil reserves reflect current output, bombing drag, and operational burn."
	snapshot.Domains["raw_materials_energy"]["oil_stockpile"] = oilMetric

	var events []model.Event
	events = append(events, model.Event{
		ID:       fmt.Sprintf("fuel_balance_%s", strings.ReplaceAll(date, "-", "")),
		Date:     date,
		Category: "economy",
		Summary:  fmt.Sprintf("Fuel balance resolved at %.2f after applying output, bombing, and operational burn.", oilMetric.Value),
	})

	if oilMetric.Value < 20 {
		supplyMetric := ensureMetric(snapshot, "logistics_fronts", "supply_status_east", model.Metric{
			Value:   0.5,
			Unit:    "ratio",
			HardCap: floatPtr(1),
		})
		supplyMetric.Value = math.Max(0, supplyMetric.Value-0.04)
		supplyMetric.Summary = "Low oil reserves are now degrading eastern supply reliability."
		snapshot.Domains["logistics_fronts"]["supply_status_east"] = supplyMetric

		events = append(events, model.Event{
			ID:       fmt.Sprintf("fuel_shortfall_%s", strings.ReplaceAll(date, "-", "")),
			Date:     date,
			Category: "logistics",
			Summary:  "Oil reserve pressure is now feeding through into eastern supply performance.",
		})
	}

	return events
}

func computeRepairProgress(snapshot *model.Snapshot, date string) []model.Event {
	damageMetric := ensureMetric(snapshot, "strategic_bombing_repair", "bombing_damage_fuel", model.Metric{
		Value:   0,
		Unit:    "ratio",
		HardCap: floatPtr(1),
	})
	repair := metricValue(*snapshot, "strategic_bombing_repair", "repair_capacity_industry")
	damageMetric.Value = math.Max(0, damageMetric.Value-repair*0.03)
	damageMetric.Summary = "Bombing damage is partially offset by current repair capacity."
	snapshot.Domains["strategic_bombing_repair"]["bombing_damage_fuel"] = damageMetric

	return []model.Event{
		{
			ID:       fmt.Sprintf("repair_progress_%s", strings.ReplaceAll(date, "-", "")),
			Date:     date,
			Category: "industry",
			Summary:  fmt.Sprintf("Repair capacity reduced fuel-plant bombing damage to %.2f.", damageMetric.Value),
		},
	}
}

func validateSnapshot(snapshot *model.Snapshot, date string) []model.Event {
	var events []model.Event
	for domainName, domain := range snapshot.Domains {
		for variable, metric := range domain {
			original := metric.Value
			if metric.HardCap != nil && metric.Value > *metric.HardCap {
				metric.Value = *metric.HardCap
			}
			if metric.Value < 0 {
				metric.Value = 0
			}
			domain[variable] = metric

			if metric.Value != original {
				events = append(events, model.Event{
					ID:       fmt.Sprintf("validation_%s_%s_%s", strings.ReplaceAll(date, "-", ""), domainName, variable),
					Date:     date,
					Category: "validation",
					Summary:  fmt.Sprintf("Validation clipped %s.%s from %.2f to %.2f.", domainName, variable, original, metric.Value),
				})
			}
		}
		snapshot.Domains[domainName] = domain
	}
	return events
}

func buildCheckpoint(snapshot model.Snapshot, branch model.BranchMeta, summary string) model.Checkpoint {
	return model.Checkpoint{
		CheckpointID:     storage.NewCheckpointID(snapshot.Date),
		SnapshotID:       snapshot.SnapshotID,
		BranchID:         snapshot.BranchID,
		ParentBranchID:   branch.ParentBranchID,
		Date:             snapshot.Date,
		Mode:             snapshot.Mode,
		ActiveDirectives: snapshot.ActiveDirectives,
		Summary:          summary,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
}

func buildSitrep(snapshot model.Snapshot, assessment model.MonthlyAssessment, review *model.ContinuityReview) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# Sitrep %s\n\n", snapshot.Date))
	builder.WriteString(fmt.Sprintf("- Branch: `%s`\n", snapshot.BranchID))
	builder.WriteString(fmt.Sprintf("- Mode: `%s`\n", snapshot.Mode))
	builder.WriteString(fmt.Sprintf("- Headline: %s\n\n", assessment.SitrepHeadline))

	builder.WriteString("## Summary\n\n")
	builder.WriteString(assessment.RationaleSummary + "\n\n")

	if len(assessment.SitrepBody) > 0 {
		builder.WriteString("## Month Notes\n\n")
		for _, line := range assessment.SitrepBody {
			builder.WriteString("- " + line + "\n")
		}
		builder.WriteString("\n")
	}

	if len(assessment.DirectiveResolutions) > 0 {
		builder.WriteString("## Directives\n\n")
		for _, resolution := range assessment.DirectiveResolutions {
			builder.WriteString(fmt.Sprintf("- `%s`: `%s` - %s\n", resolution.DirectiveID, resolution.Status, resolution.Explanation))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Key Metrics\n\n")
	for _, line := range keyMetricLines(snapshot) {
		builder.WriteString("- " + line + "\n")
	}
	builder.WriteString("\n")

	if review != nil {
		builder.WriteString("## Continuity Review\n\n")
		builder.WriteString(fmt.Sprintf("- Status: `%s`\n", review.Status))
		for _, warning := range review.Warnings {
			builder.WriteString("- Warning: " + warning + "\n")
		}
		for _, note := range review.Notes {
			builder.WriteString("- Note: " + note + "\n")
		}
		builder.WriteString("\n")
	}

	if len(snapshot.RecentEvents) > 0 {
		builder.WriteString("## Recent Events\n\n")
		for _, event := range snapshot.RecentEvents {
			builder.WriteString(fmt.Sprintf("- `%s`: %s\n", event.Category, event.Summary))
		}
	}

	return builder.String()
}

func keyMetricLines(snapshot model.Snapshot) []string {
	type line struct {
		label string
	}

	var lines []line
	for domain, variables := range snapshot.Domains {
		for variable, metric := range variables {
			lines = append(lines, line{
				label: fmt.Sprintf("%s.%s = %.2f %s", domain, variable, metric.Value, metric.Unit),
			})
		}
	}

	sort.Slice(lines, func(i, j int) bool {
		return lines[i].label < lines[j].label
	})

	limit := 12
	if len(lines) < limit {
		limit = len(lines)
	}

	output := make([]string, 0, limit)
	for _, item := range lines[:limit] {
		output = append(output, item.label)
	}
	return output
}

func metricValue(snapshot model.Snapshot, domain, variable string) float64 {
	if snapshot.Domains == nil {
		return 0
	}
	if snapshot.Domains[domain] == nil {
		return 0
	}
	return snapshot.Domains[domain][variable].Value
}

func ensureMetric(snapshot *model.Snapshot, domain, variable string, fallback model.Metric) model.Metric {
	if snapshot.Domains == nil {
		snapshot.Domains = make(map[string]model.DomainState)
	}
	state := snapshot.Domains[domain]
	if state == nil {
		state = model.DomainState{}
	}
	metric, ok := state[variable]
	if !ok {
		metric = fallback
		state[variable] = metric
		snapshot.Domains[domain] = state
	}
	return metric
}

func cloneSnapshot(snapshot model.Snapshot) (model.Snapshot, error) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return model.Snapshot{}, err
	}

	var cloned model.Snapshot
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return model.Snapshot{}, err
	}
	return cloned, nil
}

func inferredMetric(variable string) model.Metric {
	switch {
	case strings.Contains(variable, "feasibility"), strings.Contains(variable, "status"), strings.Contains(variable, "pressure"), strings.Contains(variable, "intensity"), strings.Contains(variable, "access"), strings.Contains(variable, "position"), strings.Contains(variable, "capacity"), strings.Contains(variable, "damage"):
		return model.Metric{
			Unit:    "ratio",
			HardCap: floatPtr(1),
		}
	default:
		return model.Metric{
			Unit: "index",
		}
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
