# PROJECT: WW2 Counterfactual Sandbox

## Goal

Build a steerable alt-history sandbox that models the European and wider WW2 theater from September 1939 to early 1945.

The point is not to prove whether Germany could "win." The point is to let the user mess with history, steer one or more actors, pause the run, inject new directives, fork timelines, and see what comes out: collapse, stalemate, truce, coup, frozen borders, partial victory, negotiated peace, or something stranger.

This is still not a traditional game. There is no map-painting UI or tactical combat loop. The workflow is:

1. Load a baseline historical state for a given start month.
2. Apply initial scenario settings and directives.
3. Run the simulation for `1-6` months.
4. Pause automatically and inspect the current state and sitrep.
5. Inject new timestamped directives for any actor.
6. Resume the current branch or fork a new one.
7. At any point, ask for a synthesis report on what happened and why.

Historical honesty still matters. A run with historical settings and no steering should reproduce the broad shape of the real war closely enough to trust the sandbox when the timeline diverges.

---

## Product Framing

### What This Is

- A monthly strategic adjudication engine driven by an LLM.
- A sandbox for counterfactual exploration, not a game and not a pure academic model.
- A system that supports active steering mid-run, not just initial scenario setup.
- A branching timeline machine: every pause can become a new timeline.

### What This Is Not

- Not a rigid deterministic spreadsheet with no room for interpretation.
- Not a pure narrative generator with no material constraints.
- Not a "could Germany have won?" thesis machine.
- Not a system that persists chain-of-thought or hidden reasoning logs.

### Core Design Principles

1. **Steerability first.** The user must be able to intervene after any pause and redirect one or more actors.
2. **LLM as adjudicator.** The LLM is the monthly strategic reasoner. It decides what actors attempt, how frictions bite, and how ambiguous situations resolve.
3. **Hard constraints remain external.** Code enforces hard caps, conservation laws, and structural impossibilities.
4. **State needs numbers and grounded summaries.** Numeric values keep the system coherent; short textual summaries help the LLM and the human stay aligned on what those numbers mean.
5. **Persistence must be structured.** The durable artifact is a state snapshot plus ledgers and summaries, not a stored reasoning trace.
6. **Branching is native.** The system should encourage "what if I pivot here?" rather than forcing one linear playthrough.
7. **Fun matters.** The sandbox should be enjoyable to use, not just defensible on paper.

---

## Core Experience

### Main Loop

The intended loop is:

1. Start from September 1939 or another validated checkpoint.
2. Select a mode: `strict`, `plausible`, or `god`.
3. Add zero or more directives.
4. Run `N` months, where `N` is usually `1-6`.
5. Read the sitrep and inspect the updated state.
6. Decide whether to:
   - continue the current branch,
   - inject more directives,
   - fork from this checkpoint,
   - ask for a narrative synthesis.

### Why Pauses Matter

The pause is not just a convenience feature. It is the main interaction primitive.

At each pause, the user can:

- inspect state variables and summaries,
- review which directives were followed, resisted, or failed,
- add or withdraw directives,
- switch which actor they are steering,
- create multiple branches from the same checkpoint.

This makes the project feel more like an alt-history command table than a batch simulator.

---

## Play Modes

The sandbox should support three explicit modes:

| Mode | Meaning |
|------|---------|
| `strict` | The model strongly resists implausible directives. Political friction, institutional inertia, and material limits bite hard. |
| `plausible` | The default mode. Strong steering is allowed, but actors still resist choices that clash with capacity, ideology, or incentives. |
| `god` | The user can force outcomes or attempts well outside normal plausibility. The sim should still record downstream consequences and contradictions. |

Mode is part of run state and should be visible in every snapshot.

---

## State Model

### Snapshot Structure

The world is represented as a structured monthly snapshot. Every snapshot should include:

- `snapshot_id`
- `parent_snapshot_id`
- `branch_id`
- `date`
- `mode`
- `actors`
- `domains`
- `active_directives`
- `recent_events`
- `run_metadata`

### Variable Shape

Each persistent variable should carry both a numeric value and a short grounding summary.

```json
{
  "value": 0.72,
  "unit": "ratio",
  "hard_cap": 1.0,
  "summary": "Eastern Front supply is strained by rail breaks, truck losses, and fuel rationing. Offensive capacity remains limited despite local reserves.",
  "source_note": "Derived from logistics tool outputs plus monthly adjudication"
}
```

The `summary` is not hidden reasoning. It is a compact state explanation for continuity and alignment.

### Persistent Artifacts Per Month

Every monthly step should persist structured artifacts:

- `state_snapshot.json`
- `events.jsonl`
- `directive_resolution.json`
- `adjudication_record.json`
- `sitrep.md`
- `branch_meta.json`

The adjudication record should persist:

- `rationale_summary`
- `assumptions`
- `blocked_by`
- `confidence`
- `unexpected_effects`
- `tool_calls_used`

It should not persist chain-of-thought or raw internal reasoning traces.

---

## Directive System

### Why Directives Exist

Initial scenario toggles are not enough. The user wants to steer the war after seeing how the timeline evolves.

Directives are timestamped interventions attached to actors. They are the main control surface for the sandbox.

### Directive Semantics

Each directive should include:

- `id`
- `actor`
- `effective_from`
- `effective_to`
- `scope`
- `strength`
- `priority`
- `instruction`
- `notes`
- `origin` (`user`, `scenario`, or `system`)

Example:

```json
{
  "id": "dir_1942_11_germany_01",
  "actor": "germany",
  "effective_from": "1942-11",
  "effective_to": "1943-03",
  "scope": "eastern_front",
  "strength": "hard",
  "priority": 0.95,
  "instruction": "Avoid symbolic commitment at Stalingrad. Preserve 6th Army mobility and secure withdrawal routes.",
  "notes": "Accept prestige loss in exchange for force preservation.",
  "origin": "user"
}
```

### Directive Strength

| Strength | Meaning |
|----------|---------|
| `soft` | Bias the actor toward this choice, but do not force it if stronger pressures push another way. |
| `hard` | Attempt this if it is materially and politically possible within the selected mode. |
| `god` | Override normal plausibility resistance. The sim should still model side effects and secondary damage. |

### Multi-Actor Steering

The user should be able to steer any relevant actor:

- `germany`
- `uk`
- `ussr`
- `usa`
- `italy`
- `japan`
- `france`
- `neutral_bloc`
- other named sub-actors if needed

This matters because many interesting outcomes come from changing more than one side.

### Directive Resolution

Each month, each active directive should be marked as one of:

- `followed`
- `partially_followed`
- `blocked`
- `ignored`
- `expired`

Each resolution entry should include a short explanation and a list of the main blockers:

- material shortfall
- political resistance
- timing
- enemy action
- doctrine mismatch
- institutional rivalry

---

## World Model

### Scope

Germany remains the most detailed actor early on because the project began there, but the sandbox must model other major actors well enough to respond intelligently to steering.

The world state should be organized by domain rather than by one giant flat schema.

### Domain Overview

| Domain | Purpose | Example Variables |
|--------|---------|------------------|
| Raw Materials & Energy | Physical inputs and strategic bottlenecks | `steel_production`, `aluminum_production`, `synfuel_leuna`, `oil_stockpile` |
| Industrial Production | Monthly military and civilian output | `production_fighter_single`, `production_trucks`, `factory_capacity_aircraft` |
| Manpower & Labor | Recruitment, casualties, labor mobilization, forced labor | `military_manpower_total`, `military_replacements_monthly`, `women_in_workforce_level` |
| Logistics & Fronts | Supply, transport, force posture, front viability | `supply_status_east`, `truck_pool_eastern`, `rail_gauge_converted_km`, `front_position_east` |
| Air & Naval War | Air superiority, bomber defense, submarine war, escort pressure | `air_superiority_west`, `uboat_operational`, `convoy_detection_advantage` |
| Strategic Bombing & Repair | Bombing, repair cycles, dispersal, transport disruption | `bombing_damage_fuel`, `repair_capacity_industry`, `factory_dispersal_level` |
| Technology & Programs | Program stages, bottlenecks, unlock timing | `me262_stage`, `stg44_stage`, `type_xxi_stage` |
| Diplomacy & External Relations | Neutral trade, alliance cohesion, entry decisions, peace feelers | `us_entry_status`, `turkey_chromium_trade`, `negotiated_peace_feasibility` |
| Politics & Friction | Institutional resistance, regime behavior, social tolerance | see table below |

### Politics & Friction Variables

These are critical because many counterfactuals fail for political or institutional reasons rather than physical ones.

| Variable | Unit | Meaning |
|----------|------|---------|
| `hitler_interference` | 0-100 | Degree of destructive top-level interference in military and industrial decisions |
| `bureaucratic_coordination` | 0-100 | Ability of ministries, armed services, and industry to act coherently |
| `ideological_rigidity` | 0-100 | How much ideology prevents materially sensible choices |
| `elite_cohesion` | 0-100 | Regime cohesion under stress; low values raise coup, paralysis, or fragmentation risk |
| `civilian_tolerance` | 0-100 | Willingness to absorb bombing, shortages, and mobilization burdens |
| `interservice_rivalry` | 0-100 | Competition among army, navy, air force, and party organs |
| `occupation_brutality` | 0-100 | Drives resistance, sabotage, intelligence leakage, and diplomatic hardening |
| `policy_flexibility` | 0-100 | Capacity to reverse prestige decisions before they become disasters |

These variables should be numeric, but each should also carry a short summary so the LLM has context for why they are high or low.

### State Detail by Phase

Not every domain needs equal fidelity on day one.

- Phase 1 should go deep on fuel, bombing, directives, checkpoints, and political friction.
- Phase 2 should add production, logistics, and military posture.
- Phase 3 should deepen multi-actor diplomacy and campaign outcomes.

---

## Simulation Architecture

### Overview

```text
Configuration + Scenario + Active Directives
                  |
                  v
      Load Snapshot / Branch Checkpoint
                  |
                  v
            Monthly Adjudication
                  |
      +-----------+-----------+
      |                       |
      v                       v
  LLM reasoning          Tool calls / validators
      |                       |
      +-----------+-----------+
                  |
                  v
        Revised state + ledgers + sitrep
                  |
                  v
        Pause / Resume / Fork / Synthesize
```

### Monthly Step

For each month:

1. **Pre-step code**
   - load the latest snapshot,
   - activate directives whose dates apply,
   - inject exogenous time-series data,
   - prepare the compact context package,
   - validate the incoming state.

2. **LLM adjudication**
   - determine what each major actor tries to do this month,
   - reconcile active directives with political and material reality,
   - decide ambiguous outcomes,
   - produce updated summaries and a structured monthly assessment.

3. **Tool-assisted computation**
   - resource balances,
   - production ceilings,
   - logistics capacity,
   - repair and construction timelines,
   - front attrition envelopes,
   - bombing damage and recovery,
   - diplomatic trigger checks.

4. **Validation and revision**
   - enforce hard caps,
   - reject impossible allocations,
   - detect contradictions between state and events,
   - if needed, send the result back for one more adjudication pass.

5. **Persistence**
   - write the new snapshot,
   - append event and directive ledgers,
   - generate the sitrep,
   - write checkpoint metadata.

6. **Pause handling**
   - if the requested run length is exhausted, pause,
   - expose the checkpoint for continuation or branching.

### LLM Role

The LLM is the monthly adjudicator, not just a final narrator.

It should handle:

- actor intent,
- strategic adaptation,
- institutional friction,
- doctrine clashes,
- path dependence,
- plausible enemy response,
- compression of the current state into short summaries.

It should not be trusted to override hard physical constraints.

### Deterministic Tools

The LLM should have access to deterministic tools for the parts of the problem that are better expressed as code:

- `compute_fuel_balance`
- `compute_industrial_output`
- `compute_repair_progress`
- `compute_logistics_capacity`
- `compute_front_attrition_envelope`
- `compute_bombing_effects`
- `compute_trade_and_stockpile_changes`
- `validate_state`

These tools are where conservation laws, caps, and irreversible losses live.

---

## Context Strategy

Each monthly adjudication should start fresh with a compact, structured context rather than a growing chat log.

The model should receive:

- a fixed system prompt,
- the current snapshot,
- active directives,
- recent events,
- relevant exogenous series for the month,
- the current mode,
- last month's adjudication record.

The persistence mechanism is:

- numeric variables,
- short state summaries,
- ledgers,
- checkpoint history.

Not persistent hidden reasoning.

---

## Checkpoints And Branching

### Checkpoints

Every pause should produce a checkpoint. This is mandatory, not optional.

A checkpoint includes:

- snapshot pointer,
- branch id,
- parent branch id if forked,
- list of active directives,
- run mode,
- summary of the last `1-6` months,
- tags the user can edit later.

### Branching

From any checkpoint, the user should be able to:

- continue the same branch,
- fork a new branch,
- fork multiple variants with different directives,
- compare branches later.

The system should preserve lineage:

- which checkpoint was forked,
- which directives were added or removed,
- what mode was used,
- how the new branch diverged.

This is one of the main reasons to build the project in the first place.

---

## Outcome Model

The sandbox should not reduce everything to `Germany wins` or `Germany loses`.

Each synthesis should score and describe several outcome axes:

- regime survival,
- territorial control,
- military viability,
- civilian stability,
- alliance cohesion,
- industrial continuity,
- diplomatic isolation,
- negotiated peace feasibility,
- destruction level,
- long-term strategic outlook.

Interesting outcomes include:

- a stabilized eastern front,
- a delayed collapse,
- a western truce with the war continuing elsewhere,
- internal regime fracture,
- separate peace,
- negotiated ceasefire,
- frozen borders,
- partial state survival without victory.

---

## Validation Strategy

### Historical Baseline

The first test remains the historical baseline run.

With no steering and historical decisions, the sandbox should reproduce the broad historical track:

- France falls on roughly the right timeline.
- Barbarossa reaches historical limits in late 1941.
- The Stalingrad disaster emerges unless steered away.
- U-boat effectiveness collapses in 1943.
- Aircraft output surges in 1944 but fuel collapses.
- D-Day establishes a western lodgment.
- The Reich collapses in 1945.

Close enough is acceptable. The goal is credibility, not perfect replay.

### Steering Validation

The second test is whether steering behaves sensibly.

Examples:

- A `soft` directive should sometimes fail.
- A `hard` directive should succeed only when capacities and politics permit it.
- A `god` directive should produce visibly strange but still internally tracked outcomes.
- Political friction should block some "obvious" optimizations unless other variables change first.

### Branch Validation

Forking from the same checkpoint should preserve shared history cleanly and diverge only where directives, mode, or adjudication differ.

---

## Data Sources

The same source base still makes sense, but the role changes slightly: sources are not only for one Germany-centric economic model, but also for response logic and actor capability envelopes.

Key sources:

- Adam Tooze, *Wages of Destruction*
- USSBS reports
- Wagenfuhr, *Die deutsche Industrie im Kriege 1939-1945*
- Glantz on the Eastern Front
- Overy on the air war and war economy
- Harrison on comparative wartime economics
- Blair on the U-boat war
- MGFA volumes for German military and political structure

The project should keep source-linked notes for constants, time series, and major behavioral assumptions.

---

## Technology Stack

| Component | Technology |
|-----------|------------|
| Orchestration | Python |
| LLM | Configurable frontier model with strong long-context reasoning |
| State storage | JSON snapshots and JSONL ledgers |
| Constants and caps | YAML or JSON |
| Analysis | Jupyter notebooks |
| Comparison UI | Minimal web UI or TUI later |

The implementation should be vendor-agnostic enough to swap LLMs without rewriting the engine.

---

## Suggested Repository Shape

```text
data/
  exogenous/
  constants/
  baselines/

scenarios/
  historical/
  counterfactuals/

runs/
  <run_id>/
    branches/
    checkpoints/
    snapshots/
    ledgers/
    reports/

src/
  engine/
  tools/
  schemas/
  prompts/

notebooks/
```

---

## Implementation Phases

### Phase 1: Sandbox Skeleton

Build the framework before the full world model.

- snapshot schema,
- directive ledger,
- monthly run loop,
- pause/resume,
- checkpoint creation,
- branching,
- sitrep generation,
- adjudication record persistence.

Use a very small state at first if needed. The important thing is to prove the interaction loop.

### Phase 2: Fuel, Bombing, And Repair

Deepen the fuel/oil subsystem first because it is one of the clearest strategic bottlenecks and one of the best documented.

- synthetic fuel plants,
- Romanian oil,
- bombing damage,
- repair and dispersal,
- fuel allocation,
- training penalties.

### Phase 3: Production, Labor, And Logistics

Add:

- industrial capacity,
- labor mobilization,
- production mix decisions,
- rail and truck constraints,
- front supply status.

### Phase 4: Campaign Adjudication

Deepen military outcomes:

- Eastern Front,
- air defense and bombing duel,
- Atlantic war,
- western invasion response,
- campaign-level attrition logic.

### Phase 5: Diplomacy, Politics, And Multi-Actor Steering

Expand:

- neutral trade pressure,
- war-entry changes,
- alliance fracture,
- peace feelers,
- coups, paralysis, and policy reversals,
- actor-specific directive behaviors.

### Phase 6: Comparison And Synthesis

Build tools to compare branches and ask higher-level questions:

- what changed at this fork,
- which directives mattered,
- what blocked a preferred outcome,
- how each branch scored across the outcome axes.

---

## Open Questions

1. How much endogenous Allied adaptation is required before the sandbox feels convincing under aggressive steering?
2. When should the model switch from monthly to weekly resolution for major campaign windows?
3. How strong should mode differences feel in practice?
4. Should actor steering be symmetrical, or should some actors get richer directive vocabularies first?
5. What is the minimum UI needed before this becomes fun to use regularly?

---

## Bottom Line

This project should be framed as a steerable WW2 counterfactual sandbox with:

- monthly LLM adjudication,
- hard material constraints,
- short explanatory state summaries,
- structured persistent artifacts,
- timestamped multi-actor directives,
- mandatory checkpoints,
- first-class branching.

If it is enjoyable to steer and produces coherent alternate timelines, it succeeds even when the outputs are strange. Strange is part of the point.
