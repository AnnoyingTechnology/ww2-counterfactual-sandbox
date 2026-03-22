package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/config"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/engine"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/llm"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/prompts"
	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/storage"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "run":
		return runCommand(ctx, args[1:], stdout)
	case "resume":
		return resumeCommand(ctx, args[1:], stdout)
	case "fork":
		return forkCommand(ctx, args[1:], stdout)
	case "status":
		return statusCommand(ctx, args[1:], stdout)
	case "report":
		return reportCommand(ctx, args[1:], stdout)
	case "compare":
		return compareCommand(ctx, args[1:], stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	projectRoot := fs.String("project-root", ".", "project root")
	llmConfigPath := fs.String("llm-config", "", "LLM config JSON")
	runtimeConfigPath := fs.String("runtime-config", "", "runtime config JSON")
	runID := fs.String("run-id", "", "run id")
	branchID := fs.String("branch", "main", "branch id")
	mode := fs.String("mode", "", "mode")
	months := fs.Int("months", 1, "months to advance")
	date := fs.String("date", "", "start month in YYYY-MM")
	scenario := fs.String("scenario", "", "scenario JSON path")
	baseline := fs.String("baseline", "", "baseline snapshot JSON path")
	directiveFile := fs.String("directive-file", "", "directive JSON path")
	description := fs.String("description", "", "run description")
	if err := fs.Parse(args); err != nil {
		return err
	}

	service, err := newService(*projectRoot, *llmConfigPath, *runtimeConfigPath)
	if err != nil {
		return err
	}

	result, err := service.StartRun(ctx, engine.StartRunOptions{
		RunID:         *runID,
		BranchID:      *branchID,
		Date:          *date,
		Mode:          *mode,
		ScenarioPath:  *scenario,
		BaselinePath:  *baseline,
		DirectivePath: *directiveFile,
		Description:   *description,
		Months:        *months,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "run=%s branch=%s start=%s end=%s\n", result.RunID, result.BranchID, result.StartDate, result.EndDate)
	fmt.Fprintf(stdout, "checkpoint=%s snapshot=%s\n", result.LatestCheckpointID, result.LatestSnapshotID)
	fmt.Fprintf(stdout, "sitrep=%s\n", result.SitrepPath)
	if result.InterruptedByDecisionWindow {
		fmt.Fprintf(stdout, "decision_window_interrupt=%s\n", strings.Join(result.DecisionWindowIDs, ","))
	}
	return nil
}

func resumeCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	projectRoot := fs.String("project-root", ".", "project root")
	llmConfigPath := fs.String("llm-config", "", "LLM config JSON")
	runtimeConfigPath := fs.String("runtime-config", "", "runtime config JSON")
	runID := fs.String("run", "", "run id")
	branchID := fs.String("branch", "main", "branch id")
	months := fs.Int("months", 1, "months to advance")
	directiveFile := fs.String("directive-file", "", "directive JSON path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return fmt.Errorf("--run is required")
	}

	service, err := newService(*projectRoot, *llmConfigPath, *runtimeConfigPath)
	if err != nil {
		return err
	}

	result, err := service.ResumeBranch(ctx, engine.ResumeOptions{
		RunID:         *runID,
		BranchID:      *branchID,
		DirectivePath: *directiveFile,
		Months:        *months,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "run=%s branch=%s start=%s end=%s\n", result.RunID, result.BranchID, result.StartDate, result.EndDate)
	fmt.Fprintf(stdout, "checkpoint=%s snapshot=%s\n", result.LatestCheckpointID, result.LatestSnapshotID)
	fmt.Fprintf(stdout, "sitrep=%s\n", result.SitrepPath)
	if result.InterruptedByDecisionWindow {
		fmt.Fprintf(stdout, "decision_window_interrupt=%s\n", strings.Join(result.DecisionWindowIDs, ","))
	}
	return nil
}

func forkCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("fork", flag.ContinueOnError)
	projectRoot := fs.String("project-root", ".", "project root")
	runtimeConfigPath := fs.String("runtime-config", "", "runtime config JSON")
	runID := fs.String("run", "", "run id")
	fromBranch := fs.String("from-branch", "main", "source branch")
	checkpoint := fs.String("checkpoint", "", "checkpoint date YYYY-MM")
	newBranch := fs.String("new-branch", "", "new branch id")
	directiveFile := fs.String("directive-file", "", "directive JSON path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return fmt.Errorf("--run is required")
	}

	service, err := newService(*projectRoot, "", *runtimeConfigPath)
	if err != nil {
		return err
	}

	result, err := service.ForkBranch(ctx, engine.ForkOptions{
		RunID:          *runID,
		FromBranchID:   *fromBranch,
		CheckpointDate: *checkpoint,
		NewBranchID:    *newBranch,
		DirectivePath:  *directiveFile,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "run=%s new_branch=%s date=%s\n", result.RunID, result.BranchID, result.EndDate)
	fmt.Fprintf(stdout, "checkpoint=%s snapshot=%s\n", result.LatestCheckpointID, result.LatestSnapshotID)
	return nil
}

func statusCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	projectRoot := fs.String("project-root", ".", "project root")
	runtimeConfigPath := fs.String("runtime-config", "", "runtime config JSON")
	runID := fs.String("run", "", "run id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return fmt.Errorf("--run is required")
	}

	service, err := newService(*projectRoot, "", *runtimeConfigPath)
	if err != nil {
		return err
	}

	status, err := service.Status(*runID)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "run=%s mode=%s scenario=%s branches=%d\n", status.RunMeta.RunID, status.RunMeta.Mode, status.RunMeta.Scenario, len(status.BranchStatuses))
	for _, branch := range status.BranchStatuses {
		fmt.Fprintf(stdout, "branch=%s last_date=%s checkpoint=%s directives=%d\n",
			branch.BranchMeta.BranchID,
			branch.LatestSnapshot.Date,
			branch.LatestCheckpoint.CheckpointID,
			len(branch.BranchMeta.ActiveDirectives),
		)
	}
	return nil
}

func reportCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	projectRoot := fs.String("project-root", ".", "project root")
	runtimeConfigPath := fs.String("runtime-config", "", "runtime config JSON")
	runID := fs.String("run", "", "run id")
	branchID := fs.String("branch", "main", "branch id")
	date := fs.String("date", "", "report date YYYY-MM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return fmt.Errorf("--run is required")
	}

	service, err := newService(*projectRoot, "", *runtimeConfigPath)
	if err != nil {
		return err
	}

	report, err := service.Report(*runID, *branchID, *date)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, report)
	return err
}

func compareCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	projectRoot := fs.String("project-root", ".", "project root")
	runtimeConfigPath := fs.String("runtime-config", "", "runtime config JSON")
	runID := fs.String("run", "", "run id")
	left := fs.String("left", "", "left branch")
	right := fs.String("right", "", "right branch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" || *left == "" || *right == "" {
		return fmt.Errorf("--run, --left, and --right are required")
	}

	service, err := newService(*projectRoot, "", *runtimeConfigPath)
	if err != nil {
		return err
	}

	report, err := service.CompareBranches(*runID, *left, *right)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "run=%s left=%s@%s right=%s@%s\n", report.RunID, report.LeftBranch, report.LeftDate, report.RightBranch, report.RightDate)
	limit := 10
	if len(report.Differences) < limit {
		limit = len(report.Differences)
	}
	for _, diff := range report.Differences[:limit] {
		fmt.Fprintf(stdout, "%s.%s left=%.2f right=%.2f delta=%.2f\n", diff.Domain, diff.Variable, diff.Left, diff.Right, diff.Delta)
	}
	return nil
}

func newService(projectRoot, llmConfigPath, runtimeConfigPath string) (*engine.Service, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	runtimeCfg, err := config.LoadRuntimeConfig(resolveConfigPath(root, runtimeConfigPath))
	if err != nil {
		return nil, err
	}

	store := storage.New(root, runtimeCfg.RunsDir)

	adjudicator := engine.Adjudicator(engine.NewMockAdjudicator())
	if llmConfigPath != "" {
		llmCfg, err := config.LoadLLMConfig(resolveConfigPath(root, llmConfigPath))
		if err != nil {
			return nil, err
		}
		client, err := llm.NewClient(llmCfg)
		if err != nil {
			return nil, err
		}
		if client != nil {
			pack, err := prompts.Load()
			if err != nil {
				return nil, err
			}
			adjudicator = engine.NewLLMAdjudicator(client, pack, runtimeCfg.PromptSummaryLimit)
		}
	}

	return engine.NewService(store, adjudicator, runtimeCfg), nil
}

func resolveConfigPath(projectRoot, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectRoot, path)
}

func printUsage(stdout io.Writer) {
	_, _ = io.WriteString(stdout, strings.TrimSpace(`
ww2cs commands:
  run      start a run and advance it
  resume   continue an existing branch
  fork     fork a branch from a checkpoint
  status   show branch status for a run
  report   print the latest or requested sitrep
  compare  compare the latest snapshots of two branches
`)+"\n")
}
