package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/config"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/model"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/prompts"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/storage"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/timeutil"
)

type Service struct {
	store       *storage.Store
	adjudicator Adjudicator
	runtime     config.RuntimeConfig
}

type StartRunOptions struct {
	RunID         string
	BranchID      string
	Date          string
	Mode          string
	ScenarioPath  string
	BaselinePath  string
	DirectivePath string
	Description   string
	Months        int
}

type ResumeOptions struct {
	RunID         string
	BranchID      string
	DirectivePath string
	Months        int
}

type ForkOptions struct {
	RunID          string
	FromBranchID   string
	CheckpointDate string
	NewBranchID    string
	DirectivePath  string
}

type RunResult struct {
	RunID                       string
	BranchID                    string
	StartDate                   string
	EndDate                     string
	LatestCheckpointID          string
	LatestSnapshotID            string
	SitrepPath                  string
	InterruptedByDecisionWindow bool
	DecisionWindowIDs           []string
}

type StatusReport struct {
	RunMeta        model.RunMeta
	BranchStatuses []BranchStatus
}

type BranchStatus struct {
	BranchMeta       model.BranchMeta
	LatestSnapshot   model.Snapshot
	LatestCheckpoint model.Checkpoint
}

type ComparisonReport struct {
	RunID       string
	LeftBranch  string
	RightBranch string
	LeftDate    string
	RightDate   string
	Differences []MetricDifference
}

type MetricDifference struct {
	Domain   string
	Variable string
	Left     float64
	Right    float64
	Delta    float64
}

type MonthlyPromptDump struct {
	RunID         string
	BranchID      string
	SnapshotDate  string
	TargetDate    string
	Mode          string
	DetailLevel   string
	SystemPrompt  string
	UserPrompt    string
	PromptVariant string
}

func NewService(store *storage.Store, adjudicator Adjudicator, runtime config.RuntimeConfig) *Service {
	return &Service{
		store:       store,
		adjudicator: adjudicator,
		runtime:     runtime,
	}
}

func (s *Service) StartRun(ctx context.Context, opts StartRunOptions) (RunResult, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	runID := opts.RunID
	if runID == "" {
		runID = storage.NewRunID()
	}

	branchID := opts.BranchID
	if branchID == "" {
		branchID = "main"
	}

	if _, err := s.store.LoadRunMeta(runID); err == nil {
		return RunResult{}, fmt.Errorf("run %q already exists", runID)
	}

	scenario, scenarioLoaded, err := s.maybeLoadScenario(opts.ScenarioPath)
	if err != nil {
		return RunResult{}, err
	}

	directives, err := s.loadDirectives(opts.DirectivePath)
	if err != nil {
		return RunResult{}, err
	}

	if scenarioLoaded {
		directives = appendUniqueDirectives(scenario.Directives, directives)
	}

	mode := firstNonEmpty(opts.Mode, scenario.RecommendedMode, s.runtime.DefaultMode)
	baselinePath := firstNonEmpty(opts.BaselinePath, scenario.BaselineSnapshot, s.runtime.BaselineSnapshot)

	snapshot, err := s.loadStartingSnapshot(baselinePath, firstNonEmpty(opts.Date, scenario.SuggestedStartDate), mode, runID, branchID)
	if err != nil {
		return RunResult{}, err
	}
	if scenarioLoaded {
		applyAdjustments(&snapshot, scenario.StateTweaks)
	}

	runMeta := model.RunMeta{
		RunID:       runID,
		Mode:        mode,
		Scenario:    scenario.Name,
		Description: opts.Description,
		RootBranch:  branchID,
		Branches:    []string{branchID},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	branchMeta := model.BranchMeta{
		BranchID:         branchID,
		Mode:             mode,
		Scenario:         scenario.Name,
		CreatedAt:        now,
		UpdatedAt:        now,
		ActiveDirectives: directives,
		LastSnapshotID:   snapshot.SnapshotID,
		LastDate:         snapshot.Date,
	}

	snapshot.ActiveDirectives = filterActiveDirectives(directives, snapshot.Date)
	checkpoint := buildCheckpoint(snapshot, branchMeta, "Initial baseline checkpoint.")
	initialSitrep := buildSitrep(snapshot, model.MonthlyAssessment{
		AdjudicationRecord: model.AdjudicationRecord{
			Date:             snapshot.Date,
			BranchID:         branchID,
			RationaleSummary: "Initial baseline loaded. No monthly adjudication has been run yet.",
		},
		SitrepHeadline: "Initial checkpoint",
	}, nil)

	if err := s.store.EnsureRunLayout(runID, branchID); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveRunMeta(runMeta); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveBranchMeta(runID, branchMeta); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveSnapshot(runID, branchID, snapshot); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveCheckpoint(runID, branchID, checkpoint); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveSitrep(runID, branchID, snapshot.Date, initialSitrep); err != nil {
		return RunResult{}, err
	}

	if opts.Months <= 0 {
		return RunResult{
			RunID:              runID,
			BranchID:           branchID,
			StartDate:          snapshot.Date,
			EndDate:            snapshot.Date,
			LatestCheckpointID: checkpoint.CheckpointID,
			LatestSnapshotID:   snapshot.SnapshotID,
			SitrepPath:         s.store.SitrepPath(runID, branchID, snapshot.Date),
		}, nil
	}

	return s.advanceBranch(ctx, runID, branchMeta, opts.Months)
}

func (s *Service) ResumeBranch(ctx context.Context, opts ResumeOptions) (RunResult, error) {
	branchMeta, err := s.store.LoadBranchMeta(opts.RunID, opts.BranchID)
	if err != nil {
		return RunResult{}, err
	}

	directives, err := s.loadDirectives(opts.DirectivePath)
	if err != nil {
		return RunResult{}, err
	}
	if len(directives) > 0 {
		branchMeta.ActiveDirectives = appendUniqueDirectives(branchMeta.ActiveDirectives, directives)
		branchMeta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.store.SaveBranchMeta(opts.RunID, branchMeta); err != nil {
			return RunResult{}, err
		}
	}

	return s.advanceBranch(ctx, opts.RunID, branchMeta, opts.Months)
}

func (s *Service) ForkBranch(_ context.Context, opts ForkOptions) (RunResult, error) {
	sourceBranch, err := s.store.LoadBranchMeta(opts.RunID, opts.FromBranchID)
	if err != nil {
		return RunResult{}, err
	}

	checkpointDate := opts.CheckpointDate
	if checkpointDate == "" {
		latest, err := s.store.LoadLatestCheckpoint(opts.RunID, opts.FromBranchID)
		if err != nil {
			return RunResult{}, err
		}
		checkpointDate = latest.Date
	}

	checkpoint, err := s.store.LoadCheckpoint(opts.RunID, opts.FromBranchID, checkpointDate)
	if err != nil {
		return RunResult{}, err
	}

	snapshot, err := s.store.LoadSnapshot(opts.RunID, opts.FromBranchID, checkpointDate)
	if err != nil {
		return RunResult{}, err
	}

	cloned, err := cloneSnapshot(snapshot)
	if err != nil {
		return RunResult{}, err
	}

	newBranchID := opts.NewBranchID
	if newBranchID == "" {
		newBranchID = fmt.Sprintf("%s_fork_%s", opts.FromBranchID, strings.ReplaceAll(checkpointDate, "-", ""))
	}
	if _, err := s.store.LoadBranchMeta(opts.RunID, newBranchID); err == nil {
		return RunResult{}, fmt.Errorf("branch %q already exists", newBranchID)
	}

	directives, err := s.loadDirectives(opts.DirectivePath)
	if err != nil {
		return RunResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	cloned.BranchID = newBranchID
	cloned.ParentSnapshotID = snapshot.SnapshotID
	cloned.SnapshotID = storage.NewSnapshotID(cloned.Date)
	cloned.ActiveDirectives = filterActiveDirectives(appendUniqueDirectives(sourceBranch.ActiveDirectives, directives), cloned.Date)
	cloned.RunMetadata.UpdatedAt = now

	newBranch := model.BranchMeta{
		BranchID:           newBranchID,
		ParentBranchID:     opts.FromBranchID,
		ParentCheckpointID: checkpoint.CheckpointID,
		Mode:               sourceBranch.Mode,
		Scenario:           sourceBranch.Scenario,
		CreatedAt:          now,
		UpdatedAt:          now,
		LastSnapshotID:     cloned.SnapshotID,
		LastDate:           cloned.Date,
		ActiveDirectives:   appendUniqueDirectives(sourceBranch.ActiveDirectives, directives),
	}

	runMeta, err := s.store.LoadRunMeta(opts.RunID)
	if err != nil {
		return RunResult{}, err
	}
	runMeta.Branches = append(runMeta.Branches, newBranchID)
	runMeta.UpdatedAt = now

	if err := s.store.EnsureRunLayout(opts.RunID, newBranchID); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveRunMeta(runMeta); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveBranchMeta(opts.RunID, newBranch); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveSnapshot(opts.RunID, newBranchID, cloned); err != nil {
		return RunResult{}, err
	}

	forkCheckpoint := buildCheckpoint(cloned, newBranch, fmt.Sprintf("Forked from %s at %s.", opts.FromBranchID, checkpointDate))
	forkSitrep := buildSitrep(cloned, model.MonthlyAssessment{
		AdjudicationRecord: model.AdjudicationRecord{
			Date:             cloned.Date,
			BranchID:         newBranchID,
			RationaleSummary: fmt.Sprintf("Branch fork created from %s at %s.", opts.FromBranchID, checkpointDate),
		},
		SitrepHeadline: "Fork checkpoint",
	}, nil)
	if err := s.store.SaveCheckpoint(opts.RunID, newBranchID, forkCheckpoint); err != nil {
		return RunResult{}, err
	}
	if err := s.store.SaveSitrep(opts.RunID, newBranchID, cloned.Date, forkSitrep); err != nil {
		return RunResult{}, err
	}

	return RunResult{
		RunID:              opts.RunID,
		BranchID:           newBranchID,
		StartDate:          cloned.Date,
		EndDate:            cloned.Date,
		LatestCheckpointID: forkCheckpoint.CheckpointID,
		LatestSnapshotID:   cloned.SnapshotID,
		SitrepPath:         s.store.SitrepPath(opts.RunID, newBranchID, cloned.Date),
	}, nil
}

func (s *Service) Status(runID string) (StatusReport, error) {
	runMeta, err := s.store.LoadRunMeta(runID)
	if err != nil {
		return StatusReport{}, err
	}

	branches, err := s.store.ListBranches(runID)
	if err != nil {
		return StatusReport{}, err
	}

	report := StatusReport{
		RunMeta:        runMeta,
		BranchStatuses: make([]BranchStatus, 0, len(branches)),
	}

	for _, branchID := range branches {
		branchMeta, err := s.store.LoadBranchMeta(runID, branchID)
		if err != nil {
			return StatusReport{}, err
		}
		snapshot, err := s.store.LoadLatestSnapshot(runID, branchID)
		if err != nil {
			return StatusReport{}, err
		}
		checkpoint, err := s.store.LoadLatestCheckpoint(runID, branchID)
		if err != nil {
			return StatusReport{}, err
		}

		report.BranchStatuses = append(report.BranchStatuses, BranchStatus{
			BranchMeta:       branchMeta,
			LatestSnapshot:   snapshot,
			LatestCheckpoint: checkpoint,
		})
	}

	sort.Slice(report.BranchStatuses, func(i, j int) bool {
		return report.BranchStatuses[i].BranchMeta.BranchID < report.BranchStatuses[j].BranchMeta.BranchID
	})

	return report, nil
}

func (s *Service) CompareBranches(runID, leftBranchID, rightBranchID string) (ComparisonReport, error) {
	left, err := s.store.LoadLatestSnapshot(runID, leftBranchID)
	if err != nil {
		return ComparisonReport{}, err
	}
	right, err := s.store.LoadLatestSnapshot(runID, rightBranchID)
	if err != nil {
		return ComparisonReport{}, err
	}

	var differences []MetricDifference
	seen := make(map[string]struct{})

	for domain, variables := range left.Domains {
		for variable, metric := range variables {
			key := domain + "." + variable
			seen[key] = struct{}{}
			differences = append(differences, MetricDifference{
				Domain:   domain,
				Variable: variable,
				Left:     metric.Value,
				Right:    metricValue(right, domain, variable),
				Delta:    metricValue(right, domain, variable) - metric.Value,
			})
		}
	}
	for domain, variables := range right.Domains {
		for variable, metric := range variables {
			key := domain + "." + variable
			if _, ok := seen[key]; ok {
				continue
			}
			differences = append(differences, MetricDifference{
				Domain:   domain,
				Variable: variable,
				Left:     0,
				Right:    metric.Value,
				Delta:    metric.Value,
			})
		}
	}

	sort.Slice(differences, func(i, j int) bool {
		return math.Abs(differences[i].Delta) > math.Abs(differences[j].Delta)
	})

	return ComparisonReport{
		RunID:       runID,
		LeftBranch:  leftBranchID,
		RightBranch: rightBranchID,
		LeftDate:    left.Date,
		RightDate:   right.Date,
		Differences: differences,
	}, nil
}

func (s *Service) Report(runID, branchID, date string) (string, error) {
	if date == "" {
		snapshot, err := s.store.LoadLatestSnapshot(runID, branchID)
		if err != nil {
			return "", err
		}
		date = snapshot.Date
	}
	return s.store.LoadSitrep(runID, branchID, date)
}

func (s *Service) DumpMonthlyPrompt(runID, branchID, snapshotDate, detailLevel string) (MonthlyPromptDump, error) {
	if runID == "" {
		return MonthlyPromptDump{}, fmt.Errorf("run id is required")
	}
	if branchID == "" {
		branchID = "main"
	}

	branch, err := s.store.LoadBranchMeta(runID, branchID)
	if err != nil {
		return MonthlyPromptDump{}, err
	}

	var snapshot model.Snapshot
	if snapshotDate == "" {
		snapshot, err = s.store.LoadLatestSnapshot(runID, branchID)
	} else {
		snapshot, err = s.store.LoadSnapshot(runID, branchID, snapshotDate)
	}
	if err != nil {
		return MonthlyPromptDump{}, err
	}

	targetDate, err := timeutil.AddMonths(snapshot.Date, 1)
	if err != nil {
		return MonthlyPromptDump{}, err
	}

	activeDirectives := filterActiveDirectives(branch.ActiveDirectives, targetDate)
	anchors, err := s.referenceAnchorsForMonth(targetDate)
	if err != nil {
		return MonthlyPromptDump{}, err
	}
	continuityWarnings, err := s.recentContinuityWarnings(runID, branchID)
	if err != nil {
		return MonthlyPromptDump{}, err
	}

	pack, err := prompts.Load()
	if err != nil {
		return MonthlyPromptDump{}, err
	}

	systemPrompt, err := pack.Raw("system")
	if err != nil {
		return MonthlyPromptDump{}, err
	}

	level := normalizePromptDetailLevel(firstNonEmpty(detailLevel, s.runtime.PromptDetailLevel))

	userPrompt, err := renderMonthlyPrompt(pack, AdjudicationInput{
		TargetDate:         targetDate,
		CurrentSnapshot:    snapshot,
		ActiveDirectives:   activeDirectives,
		RecentEvents:       snapshot.RecentEvents,
		HistoricalAnchors:  anchors,
		ContinuityWarnings: continuityWarnings,
		Mode:               branch.Mode,
	}, level, s.runtime.PromptSummaryLimit)
	if err != nil {
		return MonthlyPromptDump{}, err
	}

	userPrompt = strings.TrimSpace(userPrompt + "\n\nTools are unavailable for this test. Return exactly one RFC8259 JSON object in assistant content. Do not emit markdown fences, <think> tags, or explanatory text outside the JSON object.")

	return MonthlyPromptDump{
		RunID:         runID,
		BranchID:      branchID,
		SnapshotDate:  snapshot.Date,
		TargetDate:    targetDate,
		Mode:          branch.Mode,
		DetailLevel:   level,
		SystemPrompt:  systemPrompt,
		UserPrompt:    userPrompt,
		PromptVariant: "monthly_json_only",
	}, nil
}

func (s *Service) advanceBranch(ctx context.Context, runID string, branch model.BranchMeta, months int) (RunResult, error) {
	current, err := s.store.LoadLatestSnapshot(runID, branch.BranchID)
	if err != nil {
		return RunResult{}, err
	}
	if months <= 0 {
		checkpoint, err := s.store.LoadLatestCheckpoint(runID, branch.BranchID)
		if err != nil {
			return RunResult{}, err
		}
		return RunResult{
			RunID:              runID,
			BranchID:           branch.BranchID,
			StartDate:          current.Date,
			EndDate:            current.Date,
			LatestCheckpointID: checkpoint.CheckpointID,
			LatestSnapshotID:   current.SnapshotID,
			SitrepPath:         s.store.SitrepPath(runID, branch.BranchID, current.Date),
		}, nil
	}

	startDate := current.Date
	var latestCheckpoint model.Checkpoint
	var interrupted bool
	var decisionWindowIDs []string

	for step := 0; step < months; step++ {
		targetDate, err := timeutil.AddMonths(current.Date, 1)
		if err != nil {
			return RunResult{}, err
		}

		activeDirectives := filterActiveDirectives(branch.ActiveDirectives, targetDate)
		anchors, _ := s.referenceAnchorsForMonth(targetDate)
		continuityWarnings, _ := s.recentContinuityWarnings(runID, branch.BranchID)

		assessment, err := s.adjudicator.AdjudicateMonth(ctx, AdjudicationInput{
			TargetDate:         targetDate,
			CurrentSnapshot:    current,
			ActiveDirectives:   activeDirectives,
			RecentEvents:       current.RecentEvents,
			HistoricalAnchors:  anchors,
			ContinuityWarnings: continuityWarnings,
			Mode:               branch.Mode,
		})
		if err != nil {
			return RunResult{}, err
		}

		next, err := cloneSnapshot(current)
		if err != nil {
			return RunResult{}, err
		}

		next.Date = targetDate
		next.SnapshotID = storage.NewSnapshotID(targetDate)
		next.ParentSnapshotID = current.SnapshotID
		next.BranchID = branch.BranchID
		next.Mode = branch.Mode
		next.ActiveDirectives = activeDirectives
		next.RunMetadata.StepsExecuted = current.RunMetadata.StepsExecuted + 1
		next.RunMetadata.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if hasGodDirective(activeDirectives) {
			next.RunMetadata.DivergedFromHistory = true
		}

		applyAdjustments(&next, assessment.ProposedChanges)
		toolEvents := append(computeFuelBalance(&next, targetDate), computeRepairProgress(&next, targetDate)...)
		toolEvents = append(toolEvents, validateSnapshot(&next, targetDate)...)
		assessment.ToolCallsUsed = appendUniqueStrings(assessment.ToolCallsUsed,
			"apply_variable_adjustments",
			"compute_fuel_balance",
			"compute_repair_progress",
			"validate_state",
		)

		next.RecentEvents = append(append([]model.Event{}, assessment.Events...), toolEvents...)
		if len(next.RecentEvents) > 10 {
			next.RecentEvents = next.RecentEvents[len(next.RecentEvents)-10:]
		}

		var review *model.ContinuityReview
		if s.runtime.ContinuityReviewEveryMonths > 0 && next.RunMetadata.StepsExecuted%s.runtime.ContinuityReviewEveryMonths == 0 {
			recentRecords, _ := s.store.LoadRecentAdjudicationRecords(runID, branch.BranchID, 4)
			recentReviews, _ := s.store.LoadRecentContinuityReviews(runID, branch.BranchID, 2)
			generated, err := s.adjudicator.ReviewContinuity(ctx, ContinuityInput{
				Date:            targetDate,
				BranchID:        branch.BranchID,
				CurrentSnapshot: next,
				RecentRecords:   recentRecords,
				RecentReviews:   recentReviews,
			})
			if err != nil {
				return RunResult{}, err
			}
			review = &generated
		}

		sitrep := buildSitrep(next, assessment, review)
		checkpoint := buildCheckpoint(next, branch, assessment.RationaleSummary)

		if err := s.store.SaveSnapshot(runID, branch.BranchID, next); err != nil {
			return RunResult{}, err
		}
		if err := s.store.AppendEvents(runID, branch.BranchID, next.RecentEvents); err != nil {
			return RunResult{}, err
		}
		if err := s.store.AppendDirectiveResolutions(runID, branch.BranchID, assessment.DirectiveResolutions); err != nil {
			return RunResult{}, err
		}
		if err := s.store.AppendAdjudicationRecord(runID, branch.BranchID, assessment.AdjudicationRecord); err != nil {
			return RunResult{}, err
		}
		if review != nil {
			if err := s.store.AppendContinuityReview(runID, branch.BranchID, *review); err != nil {
				return RunResult{}, err
			}
		}
		if err := s.store.SaveSitrep(runID, branch.BranchID, targetDate, sitrep); err != nil {
			return RunResult{}, err
		}
		if err := s.store.SaveCheckpoint(runID, branch.BranchID, checkpoint); err != nil {
			return RunResult{}, err
		}

		branch.LastSnapshotID = next.SnapshotID
		branch.LastDate = next.Date
		branch.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.store.SaveBranchMeta(runID, branch); err != nil {
			return RunResult{}, err
		}

		runMeta, err := s.store.LoadRunMeta(runID)
		if err != nil {
			return RunResult{}, err
		}
		runMeta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.store.SaveRunMeta(runMeta); err != nil {
			return RunResult{}, err
		}

		latestCheckpoint = checkpoint
		current = next

		if ids := decisionWindowHits(targetDate, anchors); s.runtime.DecisionWindowInterrupts && len(ids) > 0 {
			interrupted = true
			decisionWindowIDs = append(decisionWindowIDs, ids...)
			break
		}
	}

	return RunResult{
		RunID:                       runID,
		BranchID:                    branch.BranchID,
		StartDate:                   startDate,
		EndDate:                     current.Date,
		LatestCheckpointID:          latestCheckpoint.CheckpointID,
		LatestSnapshotID:            current.SnapshotID,
		SitrepPath:                  s.store.SitrepPath(runID, branch.BranchID, current.Date),
		InterruptedByDecisionWindow: interrupted,
		DecisionWindowIDs:           decisionWindowIDs,
	}, nil
}

func (s *Service) loadStartingSnapshot(path, requestedDate, mode, runID, branchID string) (model.Snapshot, error) {
	snapshot, err := s.store.LoadBaselineSnapshot(path)
	if err != nil {
		snapshot = defaultSnapshot(requestedDate, mode, runID, branchID)
	} else {
		snapshot.RunMetadata.SourceBaseline = path
	}

	if snapshot.Date == "" {
		snapshot.Date = firstNonEmpty(requestedDate, "1941-06")
	}
	if requestedDate != "" {
		snapshot.Date = requestedDate
	}
	snapshot.Mode = mode
	snapshot.BranchID = branchID
	snapshot.SnapshotID = storage.NewSnapshotID(snapshot.Date)
	snapshot.ParentSnapshotID = ""
	snapshot.RunMetadata.RunID = runID
	snapshot.RunMetadata.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	snapshot.RunMetadata.UpdatedAt = snapshot.RunMetadata.CreatedAt
	if snapshot.Actors == nil {
		snapshot.Actors = defaultActors()
	}
	if snapshot.Domains == nil {
		snapshot.Domains = defaultDomains()
	}
	return snapshot, nil
}

func (s *Service) maybeLoadScenario(path string) (model.Scenario, bool, error) {
	if path == "" {
		return model.Scenario{}, false, nil
	}
	scenario, err := s.store.LoadScenario(path)
	if err != nil {
		return model.Scenario{}, false, err
	}
	return scenario, true, nil
}

func (s *Service) loadDirectives(path string) ([]model.Directive, error) {
	if path == "" {
		return nil, nil
	}
	return s.store.LoadDirectives(path)
}

func (s *Service) referenceAnchorsForMonth(date string) ([]model.ReferenceTimelineEvent, error) {
	events, err := s.store.LoadReferenceTimelineEvents()
	if err != nil {
		return nil, err
	}
	var anchors []model.ReferenceTimelineEvent
	for _, event := range events {
		if timeutil.InRange(date, event.DateStart, event.DateEnd) {
			anchors = append(anchors, event)
		}
	}
	return anchors, nil
}

func (s *Service) recentContinuityWarnings(runID, branchID string) ([]string, error) {
	reviews, err := s.store.LoadRecentContinuityReviews(runID, branchID, 1)
	if err != nil {
		return nil, err
	}
	if len(reviews) == 0 {
		return nil, nil
	}
	return reviews[len(reviews)-1].Warnings, nil
}

func decisionWindowHits(date string, events []model.ReferenceTimelineEvent) []string {
	var ids []string
	for _, event := range events {
		if event.DecisionWindow && event.DateStart == date {
			ids = append(ids, event.ID)
		}
	}
	return ids
}

func hasGodDirective(directives []model.Directive) bool {
	for _, directive := range directives {
		if directive.Strength == "god" {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func appendUniqueDirectives(existing, incoming []model.Directive) []model.Directive {
	output := append([]model.Directive{}, existing...)
	seen := make(map[string]struct{}, len(existing))
	for _, directive := range existing {
		seen[directive.ID] = struct{}{}
	}
	for _, directive := range incoming {
		if _, ok := seen[directive.ID]; ok {
			continue
		}
		output = append(output, directive)
		seen[directive.ID] = struct{}{}
	}
	return output
}

func appendUniqueStrings(existing []string, incoming ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	output := append([]string{}, existing...)
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range incoming {
		if _, ok := seen[value]; ok {
			continue
		}
		output = append(output, value)
		seen[value] = struct{}{}
	}
	return output
}

func defaultSnapshot(date, mode, runID, branchID string) model.Snapshot {
	if date == "" {
		date = "1941-06"
	}
	return model.Snapshot{
		SnapshotID: storage.NewSnapshotID(date),
		BranchID:   branchID,
		Date:       date,
		Mode:       mode,
		Actors:     defaultActors(),
		Domains:    defaultDomains(),
		RunMetadata: model.RunMetadata{
			RunID:          runID,
			SourceBaseline: "generated_default",
			CreatedAt:      time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
		},
	}
}

func defaultActors() map[string]model.ActorState {
	return map[string]model.ActorState{
		"germany": {
			Summary:          "Germany is at the center of the initial slice, with Barbarossa either imminent or underway depending on the exact branch date.",
			CurrentObjective: "Pursue decisive operational gains while managing self-inflicted political and logistical friction.",
		},
		"ussr": {
			Summary:          "The USSR is under escalating pressure but retains deep manpower and recovery potential.",
			CurrentObjective: "Absorb the blow, reconstitute reserves, and trade space for time where necessary.",
		},
		"uk": {
			Summary:          "The UK remains resilient, sustains the war economically, and waits for windows to expand pressure.",
			CurrentObjective: "Continue blockade, bombing, and coalition management while remaining ready to exploit enemy overreach.",
		},
	}
}

func defaultDomains() map[string]model.DomainState {
	return map[string]model.DomainState{
		"raw_materials_energy": {
			"oil_stockpile": {
				Value:      78,
				Unit:       "stockpile_index",
				HardCap:    floatPtr(100),
				Summary:    "Fuel reserves are workable but not comfortable; sustained operations will keep drawing them down.",
				SourceNote: "Default June 1941 scaffold",
			},
			"synthetic_fuel_output": {
				Value:      12,
				Unit:       "output_index",
				HardCap:    floatPtr(20),
				Summary:    "Synthetic production is meaningful but not yet large enough to erase strategic vulnerability.",
				SourceNote: "Default June 1941 scaffold",
			},
			"romanian_oil_access": {
				Value:      0.88,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Romanian access remains strong, though politically and militarily exposed.",
				SourceNote: "Default June 1941 scaffold",
			},
		},
		"logistics_fronts": {
			"supply_status_east": {
				Value:      0.74,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Eastern supply is good enough for momentum but already vulnerable to distance and rail friction.",
				SourceNote: "Default June 1941 scaffold",
			},
			"front_position_east": {
				Value:      0.52,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "The eastern front is balanced between offensive opportunity and overstretch risk.",
				SourceNote: "Default June 1941 scaffold",
			},
			"partisan_pressure": {
				Value:      0.14,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Rear-area resistance is present but not yet dominant.",
				SourceNote: "Default June 1941 scaffold",
			},
		},
		"strategic_bombing_repair": {
			"bombing_damage_fuel": {
				Value:      0.02,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Fuel infrastructure bombing is still minor in this early slice.",
				SourceNote: "Default June 1941 scaffold",
			},
			"repair_capacity_industry": {
				Value:      0.58,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Industrial repair capacity is moderate and can offset limited damage, but not unlimited pressure.",
				SourceNote: "Default June 1941 scaffold",
			},
		},
		"diplomacy_external_relations": {
			"negotiated_peace_feasibility": {
				Value:      0.08,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Peace remains unlikely without major strategic or political shifts.",
				SourceNote: "Default June 1941 scaffold",
			},
		},
		"atrocity_repression": {
			"genocidal_policy_intensity": {
				Value:      0.48,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Radical policy is already embedded in the branch and can intensify further if directed or implied by events.",
				SourceNote: "Default June 1941 scaffold",
			},
			"occupation_brutality": {
				Value:      0.44,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Occupation behavior is already harsh and likely to create long-run political and security costs.",
				SourceNote: "Default June 1941 scaffold",
			},
			"deportation_intensity": {
				Value:      0.40,
				Unit:       "ratio",
				HardCap:    floatPtr(1),
				Summary:    "Deportation and coercive displacement are already part of the regime toolkit.",
				SourceNote: "Default June 1941 scaffold",
			},
		},
		"politics_friction": {
			"leadership_interference": {
				Value:      65,
				Unit:       "index",
				HardCap:    floatPtr(100),
				Summary:    "Senior interference is persistent enough to distort military and industrial choices.",
				SourceNote: "Default June 1941 scaffold",
			},
			"bureaucratic_coordination": {
				Value:      46,
				Unit:       "index",
				HardCap:    floatPtr(100),
				Summary:    "Coordination is workable but fragmented, with rival institutions still competing heavily.",
				SourceNote: "Default June 1941 scaffold",
			},
			"ideological_rigidity": {
				Value:      84,
				Unit:       "index",
				HardCap:    floatPtr(100),
				Summary:    "Ideology remains a major barrier to materially rational choices.",
				SourceNote: "Default June 1941 scaffold",
			},
			"elite_cohesion": {
				Value:      72,
				Unit:       "index",
				HardCap:    floatPtr(100),
				Summary:    "The regime elite remains cohesive enough to act, but cohesion is partly fear-driven.",
				SourceNote: "Default June 1941 scaffold",
			},
			"policy_flexibility": {
				Value:      24,
				Unit:       "index",
				HardCap:    floatPtr(100),
				Summary:    "Prestige choices are hard to reverse cleanly once publicly committed.",
				SourceNote: "Default June 1941 scaffold",
			},
		},
	}
}
