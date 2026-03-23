# WW2 Counterfactual Sandbox

`ww2-counterfactual-sandbox` is a steerable WW2 alternate-history sandbox.

The engine runs in monthly turns, writes structured state to disk, pauses cleanly, and lets you continue, fork, compare, and inject new directives for one or more actors.

The first implementation is CLI-first and Go-first. It defaults to a built-in mock adjudicator, but the architecture already supports OpenAI-compatible LLM APIs.

## What This Project Is

- A monthly strategic sandbox for exploring WW2 counterfactual branches.
- A system built around `run -> pause -> steer -> fork -> compare`.
- A branchable timeline engine with structured snapshots, ledgers, checkpoints, and sitreps.
- A tool that tries to model political friction, resource pressure, logistics, and atrocity/repression dynamics honestly rather than smoothing them away.
- A CLI application first, with room for a richer UI later.

## What This Project Is Not

- Not a tactical wargame.
- Not a map-painting game loop.
- Not a pure narrative generator with no state or constraints.
- Not a deterministic spreadsheet that removes all interpretation.
- Not a thesis machine narrowly focused on "could Germany have won?"
- Not a system that stores model chain-of-thought.
- Not a tactical or operational planning tool for atrocities.

## Current Status

The repository currently contains:

- a Go module and CLI binary entrypoint,
- core state, directive, checkpoint, and adjudication schemas,
- run storage and branching logic,
- a mock monthly adjudicator,
- an OpenAI-compatible LLM client path,
- baseline data and a reference timeline scaffold,
- example scenarios and a standalone directive file.

This is an engine skeleton, not a finished historical simulation.

## Repository Layout

```text
cmd/ww2cs/                     CLI entrypoint
internal/cli/                  command parsing and wiring
internal/engine/               monthly loop, mock adjudicator, deterministic tools
internal/llm/                  provider abstraction and OpenAI-compatible transport
internal/model/                core schema types
internal/storage/              run layout and JSON/JSONL persistence
internal/prompts/              prompt templates
config/                        runtime and LLM config examples
data/                          baselines and reference timeline data
scenarios/                     starting scenarios and directive bundles
runs/                          generated run artifacts
```

## Prerequisites

- Go `1.22+`

## Build

```bash
go build -o ./bin/ww2cs ./cmd/ww2cs
```

If you are working in a constrained environment where the default Go build cache is not writable, use:

```bash
GOCACHE=$(pwd)/.cache/go-build go build -o ./bin/ww2cs ./cmd/ww2cs
```

## Quick Start

Run the historical June 1941 scenario for one month with the default mock adjudicator:

```bash
./bin/ww2cs run --scenario scenarios/historical/june_1941.json --months 1
```

Check the run status:

```bash
./bin/ww2cs status --run <run_id>
```

Read the latest sitrep:

```bash
./bin/ww2cs report --run <run_id> --branch main
```

Fork a branch:

```bash
./bin/ww2cs fork --run <run_id> --from-branch main --new-branch alt_a
```

Resume a branch:

```bash
./bin/ww2cs resume --run <run_id> --branch alt_a --months 1
```

Resume a branch with a directive bundle:

```bash
./bin/ww2cs resume \
  --run <run_id> \
  --branch alt_a \
  --months 1 \
  --directive-file scenarios/directives/germany_preserve_forces_1941.json
```

Compare two branches:

```bash
./bin/ww2cs compare --run <run_id> --left main --right alt_a
```

## Using an OpenAI-Compatible Provider

The CLI defaults to the mock adjudicator if no LLM config is supplied.

To use a real model:

1. Copy or adapt [config/llm/openai_compatible.example.json](/home/julien/Documents/Scripts/WW2_AlternativeEnding/config/llm/openai_compatible.example.json).
2. If your provider needs authentication, set the API key environment variable named in that config.
3. Pass the config file to `run` or `resume`.

`timeout_seconds` is provider-request timeout in seconds.
Set `timeout_seconds` to `-1` if you want no client-side timeout for very slow reasoning-heavy models.

Example:

```bash
export OPENAI_API_KEY=...
./bin/ww2cs run \
  --llm-config config/llm/openai_compatible.example.json \
  --scenario scenarios/historical/june_1941.json \
  --months 1
```

### LM Studio Local Test

This repo also ships a localhost config for LM Studio:

- [config/llm/lm_studio.local.json](/home/julien/Documents/Scripts/WW2_AlternativeEnding/config/llm/lm_studio.local.json)

If LM Studio is exposing an OpenAI-compatible server on `http://localhost:1234/v1` with model `qwen3.5-2b`, run:

First, do a lightweight plumbing check:

```bash
./bin/ww2cs llm-check --llm-config config/llm/lm_studio.local.json
```

Then, if you want to try the full monthly adjudication path:

```bash
./bin/ww2cs run \
  --llm-config config/llm/lm_studio.local.json \
  --scenario scenarios/historical/june_1941.json \
  --months 1
```

This local config does not require an API key.
It also uses `response_format_type: "text"` because LM Studio rejects OpenAI's older `json_object` response-format mode.
Very small local models may pass `llm-check` but still fail the full `run` command because the monthly adjudication schema is much heavier than a simple JSON connectivity test.

For remote reasoning-heavy models, expect long waits to be normal.
The client can be configured with `timeout_seconds: -1` so multi-minute generations are not cut off locally.

## Generated Artifacts

Each run writes structured data under `runs/<run_id>/branches/<branch_id>/`, including:

- snapshots,
- checkpoints,
- ledgers,
- sitreps.

The engine is designed so that the state on disk, not a chat log, is the durable history of a branch.

## Historical Framing

This project is meant to model WW2 honestly, including genocide, terror, deportation, occupation brutality, and political repression where the branch state implies them.

That honesty is part of the simulation goal. It is not a request to turn the system into a how-to tool.

## Reference Documents

- [PROJECT.md](/home/julien/Documents/Scripts/WW2_AlternativeEnding/PROJECT.md): full design spec
- [REFERENCE_TIMELINE.md](/home/julien/Documents/Scripts/WW2_AlternativeEnding/REFERENCE_TIMELINE.md): human-readable historical anchor

## Development Check

Format and verify the module with:

```bash
gofmt -w cmd/ww2cs/main.go $(find internal -name '*.go' | sort)
go test ./...
```
