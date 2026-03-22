# AGENTS

This file is for coding agents and contributors working in this repository.

## Project Summary

This repository implements a steerable WW2 counterfactual sandbox.

The core loop is:

1. load a baseline snapshot,
2. run one or more monthly turns,
3. pause,
4. inspect state and sitreps,
5. inject directives,
6. continue or fork,
7. compare branches later.

The point is branch exploration, not proving a single thesis.

## Non-Negotiable Design Intent

- Monthly strategic resolution only.
- CLI-first.
- Go-first, stdlib-first where practical.
- Structured persistence on disk; no persisted chain-of-thought.
- OpenAI-compatible LLM integration as the first provider abstraction target.
- Honest handling of WW2 atrocity, repression, and radical ideology when the branch state implies them.
- No user-facing detailed operational guidance for atrocities.

## Source Of Truth

Read these first before changing behavior:

- `PROJECT.md`
- `README.md`
- `REFERENCE_TIMELINE.md`

Then inspect the implementation:

- `internal/model/`
- `internal/storage/`
- `internal/engine/`
- `internal/cli/`

## Important Implementation Files

- `internal/model/types.go`: core schema types
- `internal/storage/storage.go`: on-disk run layout and JSON/JSONL helpers
- `internal/engine/service.go`: run/resume/fork/status/report/compare flow
- `internal/engine/mock.go`: built-in mock adjudicator
- `internal/engine/adjudicator.go`: prompt projection and LLM-backed adjudicator
- `internal/llm/openai_compatible.go`: OpenAI-compatible HTTP client
- `internal/prompts/templates/`: prompt pack

## Current Expectations

When making changes:

- preserve the `run -> pause -> steer -> fork -> compare` workflow,
- keep persistence structured and inspectable,
- prefer small deterministic helpers for hard constraints,
- keep the LLM layer provider-agnostic,
- make new state fields explicit in the snapshot schema,
- update scenario, baseline, or reference files when behavior depends on them.

## Commands

Build:

```bash
go build -o ./bin/ww2cs ./cmd/ww2cs
```

Run the default skeleton:

```bash
./bin/ww2cs run --scenario scenarios/historical/june_1941.json --months 1
```

Format and verify:

```bash
gofmt -w cmd/ww2cs/main.go $(find internal -name '*.go' | sort)
go test ./...
```

If the environment blocks the default Go build cache:

```bash
GOCACHE=$(pwd)/.cache/go-build go test ./...
```

## Agent Notes

- The mock adjudicator is intentional and useful. Do not remove it just because a real LLM path exists.
- Branch artifacts under `runs/` are generated outputs and should stay out of version control.
- The sample directive bundles under `scenarios/` are part of the developer ergonomics and should stay usable.
- If you change prompt requirements, update both the templates and the surrounding engine assumptions.
