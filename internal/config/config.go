package config

import (
	"encoding/json"
	"os"
)

type LLMConfig struct {
	Provider       string  `json:"provider"`
	BaseURL        string  `json:"base_url,omitempty"`
	APIKeyEnv      string  `json:"api_key_env,omitempty"`
	Model          string  `json:"model,omitempty"`
	TimeoutSeconds int     `json:"timeout_seconds,omitempty"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
}

type RuntimeConfig struct {
	RunsDir                     string `json:"runs_dir"`
	DefaultMode                 string `json:"default_mode"`
	BaselineSnapshot            string `json:"baseline_snapshot"`
	CheckpointEveryMonth        bool   `json:"checkpoint_every_month"`
	DecisionWindowInterrupts    bool   `json:"decision_window_interrupts"`
	ContinuityReviewEveryMonths int    `json:"continuity_review_every_months"`
	PromptSummaryLimit          int    `json:"prompt_summary_limit"`
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		RunsDir:                     "runs",
		DefaultMode:                 "plausible",
		BaselineSnapshot:            "data/baselines/june_1941_germany_ussr.json",
		CheckpointEveryMonth:        true,
		DecisionWindowInterrupts:    true,
		ContinuityReviewEveryMonths: 3,
		PromptSummaryLimit:          12,
	}
}

func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Provider:       "mock",
		TimeoutSeconds: 60,
		MaxTokens:      2400,
		Temperature:    0.3,
	}
}

func LoadLLMConfig(path string) (LLMConfig, error) {
	if path == "" {
		return DefaultLLMConfig(), nil
	}

	var cfg LLMConfig
	if err := loadJSON(path, &cfg); err != nil {
		return LLMConfig{}, err
	}
	if cfg.Provider == "" {
		cfg.Provider = "mock"
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 60
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 2400
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}
	return cfg, nil
}

func LoadRuntimeConfig(path string) (RuntimeConfig, error) {
	if path == "" {
		return DefaultRuntimeConfig(), nil
	}

	cfg := DefaultRuntimeConfig()
	if err := loadJSON(path, &cfg); err != nil {
		return RuntimeConfig{}, err
	}
	if cfg.RunsDir == "" {
		cfg.RunsDir = "runs"
	}
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = "plausible"
	}
	if cfg.ContinuityReviewEveryMonths == 0 {
		cfg.ContinuityReviewEveryMonths = 3
	}
	if cfg.PromptSummaryLimit == 0 {
		cfg.PromptSummaryLimit = 12
	}
	return cfg, nil
}

func loadJSON(path string, out any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}
