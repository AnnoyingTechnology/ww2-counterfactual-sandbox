package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/model"
)

type Store struct {
	projectRoot string
	runsDir     string
}

func New(projectRoot, runsDir string) *Store {
	if runsDir == "" {
		runsDir = "runs"
	}
	return &Store{
		projectRoot: projectRoot,
		runsDir:     runsDir,
	}
}

func (s *Store) ProjectRoot() string {
	return s.projectRoot
}

func (s *Store) RunsRoot() string {
	return filepath.Join(s.projectRoot, s.runsDir)
}

func (s *Store) RunPath(runID string) string {
	return filepath.Join(s.RunsRoot(), runID)
}

func (s *Store) BranchPath(runID, branchID string) string {
	return filepath.Join(s.RunPath(runID), "branches", branchID)
}

func (s *Store) BranchMetaPath(runID, branchID string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "branch_meta.json")
}

func (s *Store) RunMetaPath(runID string) string {
	return filepath.Join(s.RunPath(runID), "run_meta.json")
}

func (s *Store) SnapshotPath(runID, branchID, date string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "snapshots", date+".json")
}

func (s *Store) CheckpointPath(runID, branchID, date string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "checkpoints", date+".json")
}

func (s *Store) SitrepPath(runID, branchID, date string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "reports", date+"-sitrep.md")
}

func (s *Store) EventsLedgerPath(runID, branchID string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "ledgers", "events.jsonl")
}

func (s *Store) DirectiveResolutionLedgerPath(runID, branchID string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "ledgers", "directive_resolution.jsonl")
}

func (s *Store) AdjudicationLedgerPath(runID, branchID string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "ledgers", "adjudication_record.jsonl")
}

func (s *Store) ContinuityLedgerPath(runID, branchID string) string {
	return filepath.Join(s.BranchPath(runID, branchID), "ledgers", "continuity_review.jsonl")
}

func (s *Store) EnsureRunLayout(runID, branchID string) error {
	paths := []string{
		s.RunsRoot(),
		s.RunPath(runID),
		filepath.Join(s.RunPath(runID), "branches"),
		s.BranchPath(runID, branchID),
		filepath.Join(s.BranchPath(runID, branchID), "snapshots"),
		filepath.Join(s.BranchPath(runID, branchID), "checkpoints"),
		filepath.Join(s.BranchPath(runID, branchID), "reports"),
		filepath.Join(s.BranchPath(runID, branchID), "ledgers"),
	}

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveRunMeta(meta model.RunMeta) error {
	return writeJSON(s.RunMetaPath(meta.RunID), meta)
}

func (s *Store) LoadRunMeta(runID string) (model.RunMeta, error) {
	var meta model.RunMeta
	if err := readJSON(s.RunMetaPath(runID), &meta); err != nil {
		return model.RunMeta{}, err
	}
	return meta, nil
}

func (s *Store) SaveBranchMeta(runID string, meta model.BranchMeta) error {
	return writeJSON(s.BranchMetaPath(runID, meta.BranchID), meta)
}

func (s *Store) LoadBranchMeta(runID, branchID string) (model.BranchMeta, error) {
	var meta model.BranchMeta
	if err := readJSON(s.BranchMetaPath(runID, branchID), &meta); err != nil {
		return model.BranchMeta{}, err
	}
	return meta, nil
}

func (s *Store) SaveSnapshot(runID, branchID string, snapshot model.Snapshot) error {
	return writeJSON(s.SnapshotPath(runID, branchID, snapshot.Date), snapshot)
}

func (s *Store) LoadSnapshot(runID, branchID, date string) (model.Snapshot, error) {
	var snapshot model.Snapshot
	if err := readJSON(s.SnapshotPath(runID, branchID, date), &snapshot); err != nil {
		return model.Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) LoadLatestSnapshot(runID, branchID string) (model.Snapshot, error) {
	files, err := listJSONMonthFiles(filepath.Join(s.BranchPath(runID, branchID), "snapshots"))
	if err != nil {
		return model.Snapshot{}, err
	}
	if len(files) == 0 {
		return model.Snapshot{}, errors.New("no snapshots found")
	}
	return s.LoadSnapshot(runID, branchID, strings.TrimSuffix(files[len(files)-1], ".json"))
}

func (s *Store) SaveCheckpoint(runID, branchID string, checkpoint model.Checkpoint) error {
	return writeJSON(s.CheckpointPath(runID, branchID, checkpoint.Date), checkpoint)
}

func (s *Store) LoadCheckpoint(runID, branchID, date string) (model.Checkpoint, error) {
	var checkpoint model.Checkpoint
	if err := readJSON(s.CheckpointPath(runID, branchID, date), &checkpoint); err != nil {
		return model.Checkpoint{}, err
	}
	return checkpoint, nil
}

func (s *Store) LoadLatestCheckpoint(runID, branchID string) (model.Checkpoint, error) {
	files, err := listJSONMonthFiles(filepath.Join(s.BranchPath(runID, branchID), "checkpoints"))
	if err != nil {
		return model.Checkpoint{}, err
	}
	if len(files) == 0 {
		return model.Checkpoint{}, errors.New("no checkpoints found")
	}
	return s.LoadCheckpoint(runID, branchID, strings.TrimSuffix(files[len(files)-1], ".json"))
}

func (s *Store) SaveSitrep(runID, branchID, date, body string) error {
	path := s.SitrepPath(runID, branchID, date)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func (s *Store) LoadSitrep(runID, branchID, date string) (string, error) {
	payload, err := os.ReadFile(s.SitrepPath(runID, branchID, date))
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (s *Store) AppendEvents(runID, branchID string, events []model.Event) error {
	return appendJSONL(s.EventsLedgerPath(runID, branchID), events)
}

func (s *Store) AppendDirectiveResolutions(runID, branchID string, resolutions []model.DirectiveResolution) error {
	return appendJSONL(s.DirectiveResolutionLedgerPath(runID, branchID), resolutions)
}

func (s *Store) AppendAdjudicationRecord(runID, branchID string, record model.AdjudicationRecord) error {
	return appendJSONL(s.AdjudicationLedgerPath(runID, branchID), []model.AdjudicationRecord{record})
}

func (s *Store) AppendContinuityReview(runID, branchID string, review model.ContinuityReview) error {
	return appendJSONL(s.ContinuityLedgerPath(runID, branchID), []model.ContinuityReview{review})
}

func (s *Store) LoadRecentAdjudicationRecords(runID, branchID string, limit int) ([]model.AdjudicationRecord, error) {
	var records []model.AdjudicationRecord
	if err := readJSONL(s.AdjudicationLedgerPath(runID, branchID), &records); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if limit > 0 && len(records) > limit {
		return records[len(records)-limit:], nil
	}
	return records, nil
}

func (s *Store) LoadRecentContinuityReviews(runID, branchID string, limit int) ([]model.ContinuityReview, error) {
	var reviews []model.ContinuityReview
	if err := readJSONL(s.ContinuityLedgerPath(runID, branchID), &reviews); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if limit > 0 && len(reviews) > limit {
		return reviews[len(reviews)-limit:], nil
	}
	return reviews, nil
}

func (s *Store) ListBranches(runID string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.RunPath(runID), "branches"))
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, entry := range entries {
		if entry.IsDir() {
			branches = append(branches, entry.Name())
		}
	}
	sort.Strings(branches)
	return branches, nil
}

func (s *Store) LoadScenario(path string) (model.Scenario, error) {
	var scenario model.Scenario
	if err := readJSON(filepath.Join(s.projectRoot, path), &scenario); err != nil {
		return model.Scenario{}, err
	}
	return scenario, nil
}

func (s *Store) LoadDirectives(path string) ([]model.Directive, error) {
	var directives []model.Directive
	if err := readJSON(filepath.Join(s.projectRoot, path), &directives); err != nil {
		return nil, err
	}
	return directives, nil
}

func (s *Store) LoadBaselineSnapshot(path string) (model.Snapshot, error) {
	var snapshot model.Snapshot
	if err := readJSON(filepath.Join(s.projectRoot, path), &snapshot); err != nil {
		return model.Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) LoadReferenceTimelineEvents() ([]model.ReferenceTimelineEvent, error) {
	path := filepath.Join(s.projectRoot, "data", "reference_timeline", "events.jsonl")
	var events []model.ReferenceTimelineEvent
	if err := readJSONL(path, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) LoadReferenceTimelineCheckpoints() ([]model.ReferenceTimelineCheckpoint, error) {
	path := filepath.Join(s.projectRoot, "data", "reference_timeline", "checkpoints.json")
	var checkpoints []model.ReferenceTimelineCheckpoint
	if err := readJSON(path, &checkpoints); err != nil {
		return nil, err
	}
	return checkpoints, nil
}

func NewRunID() string {
	now := time.Now().UTC()
	return fmt.Sprintf("run_%s_%d", now.Format("20060102T150405Z"), now.UnixNano())
}

func NewSnapshotID(date string) string {
	return fmt.Sprintf("snapshot_%s_%d", strings.ReplaceAll(date, "-", ""), time.Now().UTC().UnixNano())
}

func NewCheckpointID(date string) string {
	return fmt.Sprintf("checkpoint_%s_%d", strings.ReplaceAll(date, "-", ""), time.Now().UTC().UnixNano())
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

func readJSON(path string, out any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func appendJSONL[T any](path string, records []T) error {
	if len(records) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}

func readJSONL[T any](path string, out *[]T) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var records []T
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			return err
		}
		records = append(records, item)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	*out = records
	return nil
}

func listJSONMonthFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".json" {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}
