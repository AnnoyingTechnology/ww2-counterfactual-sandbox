# PROJECT: WW2 Counterfactual Simulator

## Goal

Build a simulation engine that models Germany's strategic position from September 1939 to early 1945. The user defines a set of initial conditions and decision overrides (counterfactuals), the engine runs month-by-month to produce a final world state, and a synthesis step produces a narrative assessment of whether Germany could have "won" (or at least avoided unconditional surrender) under those conditions.

This is **not a game**. There is no interactive loop. The workflow is:

1. Load a baseline state representing historical September 1939.
2. Apply user tweaks: toggle decisions (e.g., "no Barbarossa", "StG 44 approved in 1941"), adjust sliders (e.g., "60% of fighter production shifted to Me 262 from mid-1943").
3. Run the simulation forward (~65 monthly steps).
4. Inspect the final state. Optionally, run a synthesis LLM call to produce a narrative summary.

The simulation must be **honest**: when fed historical inputs with no tweaks, it should reproduce historical outcomes (production curves, front positions, resource availability, collapse timeline) within reasonable tolerance. Only then do counterfactual runs have meaning.

---

## World Model

### Design Philosophy

The world is represented as a structured JSON state vector updated monthly. Each variable has a numeric value and a **text trace** — a short natural-language summary of its current state and recent evolution. The text trace is the key innovation: it lets the LLM reasoning engine understand *why* a number is what it is, and it provides auditability for the human operator.

All numeric values are grounded in physical constraints (hard caps) that the LLM cannot override. The LLM reasons about *what fraction of the physical maximum is realized* given current conditions. Code enforces conservation laws and caps.

Allied nations are modeled as **exogenous forcing functions**. Their production, strategy, and force deployment follow known historical curves. The user does not control them. This is both a simplification and historically defensible: the Allies were broadly pursuing maximum mobilization from their respective entry points onward.

### State Vector — Domain Breakdown

The state is organized into domains. Each domain contains variables with:

- `value`: numeric (float or int), in stated units
- `unit`: string
- `hard_cap`: maximum physically possible (nullable — some variables are unbounded)
- `trace`: string, ≤150 tokens, cumulative summary of current state and key recent changes

#### 1. Raw Materials & Energy

These represent extraction, production, and import of fundamental inputs. Most have geological or industrial hard caps.

| Variable | Unit | Hard Cap Source | Notes |
|----------|------|-----------------|-------|
| `steel_production` | tons/month | Blast furnace capacity (~2.3M t/mo peak) | Function of ore (Sweden, Lorraine, domestic), coking coal, labor, furnace status |
| `aluminum_production` | tons/month | Smelter capacity + electricity | Critical for aircraft. Bauxite from Hungary, France, domestic |
| `copper_supply` | tons/month | Import-dependent (Sweden, Balkans, stockpile) | Ammunition, electronics |
| `chromium_supply` | tons/month | Turkey + stockpile (≈18 months at 1939 rate) | Armor steel hardening. Stockpile depletion is a hard clock |
| `tungsten_supply` | tons/month | Portugal, Spain, stockpile | Armor-piercing rounds, machine tools |
| `rubber_production` | tons/month | Buna synthetic plant capacity | Natural rubber stockpile negligible after ~1940 |
| `explosives_production` | tons/month | Chemical plant capacity (nitric acid) | Function of chemical industry status |
| `electricity_generation` | GWh/month | Power plant capacity, coal supply | Constrains aluminum, synthetic fuel, chemical production |

**Oil & Fuel sub-model** (critical — deserves per-facility granularity):

| Variable | Unit | Hard Cap | Notes |
|----------|------|----------|-------|
| `synfuel_leuna` | tons/month | 55,000 | Individually targetable by bombing |
| `synfuel_politz` | tons/month | 45,000 | Individually targetable |
| `synfuel_brux` | tons/month | 30,000 | Sudeten, operational from 1943 |
| `synfuel_other` | tons/month | ~80,000 aggregate | ~9 smaller plants |
| `oil_romania` | tons/month | ~270,000 | Ploești. Bombing from mid-1943, Soviet capture Oct 1944 |
| `oil_hungary` | tons/month | ~15,000 | Minor but non-trivial |
| `oil_austria` | tons/month | ~10,000 | Zistersdorf field |
| `oil_stockpile` | tons | Finite, ~2-3M tons Sept 1939 | Drawdown rate = consumption - production |
| `fuel_total_available` | tons/month | Computed | Sum of above minus processing losses |
| `fuel_allocation_military` | tons/month | — | Split across fronts, training, navy, air |
| `fuel_allocation_civilian` | tons/month | — | Industry, transport, agriculture |
| `fuel_allocation_training` | tons/month | — | Critically: Luftwaffe pilot training hours |

#### 2. Industrial Production

Output rates for major military equipment categories. Each is a function of raw material inputs, factory capacity (which can be expanded, dispersed, or bombed), labor, and machine tools.

| Variable | Unit | Notes |
|----------|------|-------|
| `production_fighter_single` | units/month | Bf 109, Fw 190 |
| `production_fighter_jet` | units/month | Me 262. Requires Jumo 004 engines (chromium-dependent, unreliable) |
| `production_bomber` | units/month | He 111, Ju 88, Do 217 |
| `production_tank_pziv` | units/month | Workhorse. Reliable, cheap |
| `production_tank_panther` | units/month | Superior but complex. Transmission problems until mid-1944 |
| `production_tank_tiger` | units/month | Resource sinkhole. ~50/month peak. Arguably net negative |
| `production_assault_gun` | units/month | StuG III/IV. Cheaper than tanks, very effective defensively |
| `production_uboat_vii` | units/month | Standard. Increasingly obsolete by 1943 |
| `production_uboat_xxi` | units/month | Revolutionary but late. Modular construction from ~1944 |
| `production_v1` | units/month | Cheap cruise missile. Limited accuracy. Consumes little relative to V-2 |
| `production_v2` | units/month | Extremely expensive for negligible military impact. Historical resource drain |
| `production_small_arms` | units/month | Aggregate. Includes rifle (K98k vs. StG 44 mix), MG42, etc. |
| `production_ammunition` | tons/month | Aggregate by caliber class |
| `production_trucks` | units/month | Chronically insufficient. Germany never matched Allied truck production |
| `production_locomotives` | units/month | Rail transport capacity growth |
| `production_artillery` | units/month | Aggregate |
| `production_radar_sets` | units/month | Defensive (Würzburg, Freya) and naval |
| `industrial_output_index` | index (1939=100) | Aggregate. Tracks Speer-era rationalization, bombing damage, dispersal |

**Factory status** should be modeled at a coarser level — not per-factory, but per-category:

| Variable | Unit | Notes |
|----------|------|-------|
| `factory_capacity_aircraft` | index | Affected by bombing, dispersal, expansion |
| `factory_capacity_armor` | index | — |
| `factory_capacity_naval` | index | — |
| `factory_capacity_ammunition` | index | — |
| `factory_capacity_chemical` | index | Synthetic fuel, explosives, rubber |
| `factory_dispersal_level` | 0-100 | Higher = more resilient to bombing, less efficient |

#### 3. Manpower & Labor

| Variable | Unit | Notes |
|----------|------|-------|
| `military_manpower_total` | thousands | In uniform. ~4.5M Sept 1939, peaked ~9.5M |
| `military_casualties_cumulative` | thousands | KIA + permanent disability. Non-recoverable |
| `military_pow_cumulative` | thousands | Mostly non-recoverable (especially after Stalingrad) |
| `military_replacements_monthly` | thousands/month | Training pipeline output |
| `civilian_labor_force` | thousands | Available for industry + agriculture |
| `forced_labor_count` | thousands | POWs, *Ostarbeiter*, concentration camp. Peaks ~7-8M |
| `forced_labor_productivity` | 0-1.0 | Fraction of free worker output. ~0.5-0.8 depending on conditions |
| `women_workforce_participation` | 0-1.0 | Historically low (vs. UK/USSR). Slider candidate |
| `pilot_training_hours_monthly` | hours | Function of fuel allocation. Historically collapsed by 1944 |
| `skilled_worker_availability` | index | Drafting skilled workers to the front degrades production |

#### 4. Military — Per Front

Five theaters: **Western**, **Eastern**, **Mediterranean/Africa**, **Atlantic** (naval), **Air Defense** (strategic bombing defense). Each front carries:

| Variable | Unit | Notes |
|----------|------|-------|
| `divisions_total` | count | By type: infantry, panzer, mechanized, parachute |
| `division_equipment_level` | 0-1.0 | Fraction of TOE (table of organization & equipment) |
| `division_experience` | 0-1.0 | Degrades with casualties and green replacements |
| `supply_status` | 0-1.0 | 1.0 = fully supplied. Below 0.5 = combat effectiveness halved |
| `front_position` | index | Abstract territorial control. 100 = maximum historical advance, 0 = homeland border |
| `fortification_level` | 0-100 | Defensive works. Atlantic Wall, Siegfried Line, Ostwall |
| `air_superiority` | -1.0 to 1.0 | Negative = Allied dominance. Affects ground operations |

**Eastern Front specifics:**
| Variable | Unit | Notes |
|----------|------|-------|
| `rail_gauge_converted_km` | km | Soviet broad gauge → standard. Limits logistics depth |
| `truck_pool_eastern` | count | Degrades fast (mud, cold, distance) |
| `partisan_activity` | 0-100 | Disrupts supply lines. Historically severe from 1942 |

**Atlantic specifics:**
| Variable | Unit | Notes |
|----------|------|-------|
| `uboat_operational` | count | Active boats at sea |
| `uboat_losses_monthly` | count | Spiked catastrophically May 1943 |
| `allied_shipping_sunk` | tons/month | Output metric |
| `convoy_detection_advantage` | -1.0 to 1.0 | Radar/Enigma balance. Flipped mid-1943 |

#### 5. Strategic Bombing (Exogenous + Response)

| Variable | Unit | Notes |
|----------|------|-------|
| `allied_bomber_sorties_monthly` | count | Exogenous. Known escalation curve |
| `bombing_target_priority` | enum | Exogenous. Oil / transport / industry / cities — follows historical USAAF/RAF strategy |
| `luftwaffe_day_fighters_defense` | count | Player-allocated (trade-off with fronts) |
| `flak_batteries` | count | Resource-intensive (88mm guns, ammunition, crews) |
| `bombing_damage_fuel` | 0-1.0 | Reduction in synthetic fuel capacity |
| `bombing_damage_transport` | 0-1.0 | Rail network degradation |
| `bombing_damage_industry` | 0-1.0 | General production degradation |
| `civilian_morale` | 0-100 | Affected by bombing, losses, food availability |

#### 6. Technology & Research

Modeled as programs with progress levels, resource costs, and unlock dates. Each has historical timelines that can be accelerated or delayed.

| Program | Levels | Historical Completion | Notes |
|---------|--------|----------------------|-------|
| Me 262 jet fighter | Design → Prototype → Testing → Production → Deployment | Mid-1944 | Jumo 004 engine reliability is the bottleneck. Chromium supply affects engine life |
| StG 44 | Design → Testing → Limited production → Mass production | Late 1943 | Hitler's obstruction is a toggle. Requires 7.92x33 Kurz ammo supply chain |
| Type XXI U-boat | Design → Modular construction → Sea trials → Operational | Never (war ended) | Potentially war-changing if deployed 1943 |
| V-2 rocket | R&D → Testing → Production → Deployment | Sept 1944 | Enormous resource drain for near-zero military value. Classic misallocation |
| V-1 flying bomb | R&D → Production → Deployment | June 1944 | Much cheaper than V-2, comparable (mediocre) effect |
| Synthetic fuel expansion | Capacity planning → Construction → Online | Ongoing | Each new plant: ~18 months construction, major resource investment |
| Nuclear program | Theoretical → Experimental → Reactor → Enrichment → Weapon | Never (nowhere close) | Would require redirecting most of the electrical and chemical industry. Near-impossible |
| Radar (defensive) | Early warning → GCI → Airborne intercept | Ongoing | Historically behind UK. Affects night fighter effectiveness |
| Advanced tanks (Panther) | Design → Prototype → Production → Reliability fixes | Early 1943 | Initial reliability was terrible. Rushing deployment = higher loss rate |

#### 7. Diplomacy & External

Mostly exogenous, but some variables respond to German actions.

| Variable | Unit | Notes |
|----------|------|-------|
| `us_entry_status` | bool | Historically Dec 1941. Toggle: "no declaration of war on US" |
| `us_war_production_index` | index | Exogenous ramp. Unstoppable once engaged |
| `soviet_production_index` | index | Exogenous. Post-Urals-relocation recovery curve |
| `uk_production_index` | index | Exogenous |
| `turkey_chromium_trade` | bool | Historically stopped Aug 1944. Affected by diplomatic pressure |
| `sweden_iron_ore_trade` | bool | Historically continuous. Allied pressure variable |
| `spain_belligerence` | 0-1.0 | Franco stayed out. Toggle: "Spain enters war" (very unlikely) |
| `italy_status` | enum | Allied / Axis / Surrendered / Co-belligerent | Historically switched Sept 1943 |
| `japan_coordination` | 0-1.0 | Historically minimal. Could draw US resources to Pacific |
| `war_crimes_index` | 0-100 | Cumulative. Affects neutral diplomacy, enemy resolve, possibility of negotiated peace, partisan intensity |
| `negotiated_peace_feasibility` | 0-1.0 | Computed. Drops toward zero as war crimes rise and Allied commitment hardens |

---

## Decision Nodes (User-Configurable)

### Toggles (Binary or Branching)

| Toggle | Historical | Counterfactual | Impact |
|--------|-----------|----------------|--------|
| Barbarossa | Launched June 1941 | Delay to 1942 / Cancel / Limited (Caucasus only) | Defines the entire Eastern Front. Most consequential single decision |
| US war declaration | Dec 1941 (after Pearl Harbor) | No declaration | Removes direct US ground/air pressure on Western Front. Atlantic war continues |
| Stalingrad fixation | City assault, encirclement | Bypass, prioritize Caucasus oil fields | Saves 6th Army (~300k), potentially secures oil |
| StG 44 early approval | Blocked until 1943 | Approved 1941 | Infantry effectiveness, ammunition logistics change |
| Me 262 doctrine | Hitler insisted on bomber use | Pure fighter from the start | Air defense effectiveness. Delays but improves deployment |
| Me 262 priority | Low until late | High priority from 1942 | Accelerates timeline but competes for resources |
| Speer reforms timing | February 1942 | From 1940 (or even 1939) | Total war mobilization, production rationalization. ~2 years of lost output |
| Women in workforce | Minimal mobilization | Full mobilization (UK/USSR level) | +2-3M additional workers. Ideological conflict |
| V-2 program | Funded heavily | Cancelled / Redirected to fighters | Frees ~20,000 skilled workers + materials equivalent to ~24,000 fighters |
| Kursk | Attack (July 1943) | Defend / Skip | Saves panzer reserves. Potentially extends Eastern Front viability by months |
| Atlantic Wall strategy | Rommel (beach defense) vs. Rundstedt (mobile reserve) | Unified strategy, earlier start | D-Day outcome. Historically, the compromise was worst of both |
| Total war declaration | February 1943 (Goebbels) | From 1939 | Consumer goods production cut earlier. Civilian morale impact |
| Nuclear program | Neglected | Seriously funded | Would require massive redirection. Almost certainly too late regardless |
| Dunkirk | Halt order (historical) | No halt — attempt to destroy BEF | Potentially eliminates 300k+ experienced troops. Risky (Luftwaffe claims were inflated) |
| Norway garrison | ~400k troops tied down entire war | Reduce to minimal | Frees divisions, but risks Allied northern flank |
| Forced labor intensity | Historical escalation | Reduced (lower war crimes, less resistance) / Increased | Trade-off: production vs. resistance/sabotage/diplomatic cost |

### Sliders (Continuous)

| Slider | Range | Historical Value | Notes |
|--------|-------|-----------------|-------|
| Fighter vs. bomber production ratio | 0-100% | ~60% fighters by 1944 (was ~40% in 1940) | Earlier shift to fighters improves air defense |
| Eastern vs. Western front allocation | 0-100% | ~65-75% East from 1941 | |
| Air defense vs. front air support | 0-100% | Shifted to defense from 1943 | |
| Fuel allocation: training vs. operations | 0-100% | Training slashed from 1942 | Pilot quality collapse is a major factor |
| Synthetic fuel investment level | 0-3x historical | 1x | More plants = more fuel, but construction competes for resources |
| Panzer production mix: Pz IV / Panther / Tiger / StuG | % each | Historical evolution | Tiger is a trap. StuG is underrated. Panther is good if reliable |
| Submarine priority: Type VII vs. Type XXI | 0-100% | Type XXI from 1943 | Earlier XXI commitment is promising but construction is complex |
| Fortification investment (West) | 0-3x historical | 1x | Atlantic Wall. Competes with Eastern Front resources |

---

## Exogenous Data (Allied Forcing Functions)

These are time-series loaded from historical data, not computed by the model. The LLM receives them each month as context.

- **US war production index**: Month-by-month from Dec 1941. Liberty ships, aircraft, tanks, ammunition. Source: WPB statistical reports.
- **Soviet production**: T-34 output, artillery, aircraft. Post-Urals relocation curve. Source: Overy, Harrison.
- **UK production**: Aircraft, naval, ammunition. Source: Postan, *British War Production*.
- **Allied bomber availability**: 8th Air Force + RAF Bomber Command sortie capacity by month. Source: USSBS.
- **Allied bombing target selection**: Historical priority shifts (industry → oil → transport). Can be overridden if counterfactual changes warranting different Allied response.
- **Soviet front strength by month**: Divisions, equipment. Source: Glantz.
- **Lend-Lease deliveries to USSR**: Trucks (400k+), food, aluminum, explosives. Non-trivial contributor to Soviet logistics.

Format: CSV or JSON time series, one entry per month, Sept 1939 through May 1945.

---

## Simulation Architecture

### Overview

```
┌────────────────────────────────────────────────────┐
│  CONFIGURATION                                      │
│  baseline_state.json + scenario_overrides.json      │
│  (toggles, sliders, initial condition tweaks)       │
└───────────────────────┬────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────┐
│  INITIALIZATION                                     │
│  Code: load baseline, apply overrides, validate     │
│  Load exogenous time series                         │
└───────────────────────┬────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────┐
│  MONTHLY STEP (repeat ~65x)                         │
│                                                     │
│  1. CODE — Pre-step:                                │
│     - Inject exogenous data for this month          │
│     - Validate previous state (conservation laws,   │
│       hard caps, logical consistency)               │
│     - Prepare context payload                       │
│                                                     │
│  2. LLM CALL — Reasoning:                           │
│     Receives: system prompt + state JSON with       │
│     traces + exogenous data + scenario rules        │
│     Task: "Determine what happens this month"       │
│     Outputs:                                        │
│       - Per-variable: proposed delta with reasoning  │
│       - Updated trace summaries                     │
│       - Decision triggers (if toggle conditions met)│
│       - Monthly narrative summary                   │
│     May use tool calls for computation              │
│                                                     │
│  3. CODE — Post-step:                               │
│     - Apply deltas to numeric values                │
│     - Enforce hard caps and conservation laws       │
│     - If violations detected: re-run step with      │
│       violation feedback                            │
│     - Update state JSON                             │
│     - Append to run log                             │
│                                                     │
└───────────────────────┬────────────────────────────┘
                        │ (loop)
                        ▼
┌────────────────────────────────────────────────────┐
│  SYNTHESIS                                          │
│  Final LLM call with:                               │
│  - Final state + full trace summaries               │
│  - Key decision points from the run log             │
│  - "Assess: did Germany survive? What happened?"    │
│  Output: narrative report                           │
└────────────────────────────────────────────────────┘
```

### The LLM's Role

The LLM is the **reasoning engine**, not the calculator. It determines:

- Qualitative state changes: "Leuna is at ~60% after emergency repairs this month — the dispersal to underground facilities is 40% complete but won't be operational until March"
- Causal chains: "chromium stockpile depletion means Panther tank armor quality drops, increasing losses on the Eastern Front, which increases manpower drain"
- Decision consequences: "with StG 44 in mass production since 1942, infantry squad firepower on the Eastern Front is substantially higher, partially compensating for reduced artillery ammunition"
- Interactions the designer didn't hardcode: the model's world knowledge fills gaps

The LLM **never does arithmetic**. It proposes deltas as percentages or qualitative assessments. Code computes the numbers.

### Tool Calls Available to the LLM

The LLM can call tools during its reasoning step to get precise numbers:

```
compute_fuel_production(
  plant_status: {leuna: 0.60, politz: 0.0, brux: 0.85, ...},
  romanian_throughput: 0.85,
  hungarian_throughput: 1.0,
  stockpile_release: 20000
) → {total_tons: 382000, deficit_vs_military_demand: -145000, deficit_vs_total_demand: -210000}

compute_equipment_production(
  category: "fighter_single",
  aluminum_available: 12000,
  engine_available: 1800,
  factory_capacity: 0.9,
  labor_available: 0.85,
  machine_tools_status: 0.8
) → {units: 1620, bottleneck: "engines"}

compute_logistics_capacity(
  front: "eastern",
  rail_km_converted: 1800,
  rail_network_damage: 0.15,
  truck_pool: 42000,
  fuel_allocated: 85000
) → {max_divisions_supplied: 120, current_deployed: 155, supply_ratio: 0.77}

compute_attrition(
  front: "eastern",
  force_ratio: 0.7,
  supply_ratio: 0.77,
  season: "winter",
  terrain: "steppe",
  fortification: 0.2
) → {casualties_thousands: 85, equipment_loss_pct: 8, retreat_km: 40}

validate_state(state_json)
→ {violations: ["chromium consumption exceeds supply + stockpile drawdown by 200t"]}
```

These tools embed the hard physics of the model. Their internals are deterministic code with known constants and caps. The LLM uses them to check its reasoning against reality.

### Context Window Management

Each monthly step restarts with a fresh context. The LLM does **not** accumulate 65 months of conversation history. It sees:

- **Fixed**: System prompt, rules, variable definitions, hard caps reference (~15-20k tokens)
- **Variable**: Current state JSON with trace summaries (~30-40k tokens)
- **Monthly**: Exogenous data, last month's delta log (~5k tokens)
- **Output**: Reasoning, proposed deltas, updated traces (~10-15k tokens)

**Total per step: ~60-80k tokens.** Well within 200k limits. No drift from context overflow.

The **trace summary** per variable is the persistence mechanism. Each step, the LLM reads the current trace (e.g., "Leuna: rebuilt to 60% after April bombing, dispersal program 40% complete, operating with reduced workforce due to May raids") and updates it with this month's events. The trace is a rolling compression of history — recent events in detail, older events absorbed into the current-state description.

If a trace grows too long, the post-step code can flag it for compression on the next iteration.

### Validation & Guardrails

After each LLM step, code enforces:

1. **Hard caps**: No variable exceeds its physical maximum. Romanian oil ≤ 270k tons/month. German male population 16-45 is finite and declining.
2. **Conservation laws**: Total steel consumed across all production categories ≤ steel produced + stockpile change. Total fuel allocated ≤ fuel available. Casualties + surviving troops ≤ previous total + replacements.
3. **Monotonicity constraints**: Cumulative casualties never decrease. Chromium stockpile never increases beyond import rate. Destroyed infrastructure doesn't spontaneously regenerate.
4. **Plausibility bounds**: No single variable changes by more than ±30% in one month without exceptional justification (tagged in reasoning). Front position can't jump more than historically-calibrated maximum advance/retreat rates.

Violations trigger a **re-run** of the step with the violation explicitly flagged in the prompt: "Your previous output implied chromium consumption of 3,200t but supply + stockpile drawdown only provides 2,800t. Revise."

---

## Validation Strategy

### Historical Baseline Run

The primary validation is running the simulation with all toggles set to historical decisions, all sliders at historical values, and checking output against known data.

**Key validation checkpoints** (values to match within ~15%):

| Date | Metric | Historical Value | Source |
|------|--------|-----------------|--------|
| June 1940 | France falls | Y/N + timeline | — |
| Dec 1941 | Eastern front position | Gate of Moscow, failed | Glantz |
| Feb 1942 | German tank production | ~500/month | Tooze |
| Mid-1942 | Peak fuel production | ~650k tons/month | USSBS |
| Feb 1943 | Stalingrad surrender | ~91k POW | — |
| July 1943 | Speer production index | ~230 (1939=100) | Tooze |
| Mid-1943 | U-boat losses spike | ~40/month May 1943 | Blair |
| Mid-1944 | Peak aircraft production | ~3,000 fighters/month | USSBS |
| June 1944 | D-Day outcome | Allied beachhead established | — |
| Late 1944 | Fuel crisis | Synthetic production ~10% of peak | USSBS |
| April 1945 | Collapse | Surrender | — |

If the historical run matches these waypoints, the model's structural relationships are sound.

### Sensitivity Analysis

Once validated, systematic sensitivity analysis:

1. **Single-variable sweeps**: Toggle one decision or slider at a time, measure impact on collapse date and final state. Identifies which decisions are high-leverage.
2. **Cluster analysis**: Group correlated tweaks (e.g., "rational resource allocation" = Speer early + V-2 cancelled + fighter priority + synthetic fuel investment) and test as packages.
3. **Monte Carlo**: Add stochastic noise to the LLM's monthly assessments (run each scenario 5-10 times), measure variance. If variance is high, the model is too sensitive to LLM judgment and needs more hard constraints.

---

## Data Sources for Calibration

| Source | What it provides |
|--------|-----------------|
| Adam Tooze, *Wages of Destruction* (2006) | German war economy: production indices, resource flows, labor, finance. Best single source. |
| USSBS (US Strategic Bombing Survey, 1945-46) | Detailed production data, bombing damage assessments, fuel/oil situation, equipment output by month. Primary source, freely available. |
| Rolf-Dieter Müller, *Germany and the Second World War* (MGFA series, Vol. V/I & V/II) | Official German military history. Organization, mobilization, war economy. |
| David Glantz, *When Titans Clashed* (1995) | Eastern Front operational data: force ratios, casualties, logistics. |
| Glantz, *Barbarossa* series | Detailed logistics data for the Eastern Front |
| Richard Overy, *The Air War 1939-1945* (1980) | Production figures for all belligerents. Aircraft, engines, aluminum |
| Richard Overy, *Why the Allies Won* (1995) | Comparative economic analysis |
| Mark Harrison, *The Economics of World War II* (1998) | GDP comparisons, production data across all belligerents |
| Clay Blair, *Hitler's U-Boat War* (1996-98) | Detailed U-boat operations, losses, Allied countermeasures by month |
| M.M. Postan, *British War Production* (1952) | UK production data |
| R.J. Overy, *War and Economy in the Third Reich* (1994) | German economic policy and production decisions |
| Albert Speer, *Inside the Third Reich* (1970) | First-person account of production decisions (treat with skepticism — Speer self-aggrandizes) |

**Primary quantitative sources for time-series calibration:**
- USSBS reports (especially "The Effects of Strategic Bombing on the German War Economy", 1945)
- Wagenführ, *Die deutsche Industrie im Kriege 1939-1945* (1963) — detailed monthly production statistics
- OKW war diaries for military strength returns

---

## Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Orchestration | Python | State management, validation, tool dispatch, logging |
| LLM | Claude Sonnet (via API) | Best cost/quality for structured reasoning at this volume. Opus for synthesis step |
| State storage | JSON files | One per step for full auditability. Git-trackable |
| Tool implementations | Python functions | Deterministic, testable, version-controlled |
| Exogenous data | CSV/JSON | Pre-compiled from historical sources |
| Analysis | Jupyter notebooks | Post-run visualization, comparison across scenarios |
| Hard caps & constants | YAML/JSON config | Separate from code. Editable, documented, sourced |

**Estimated cost per run** (65 steps, Sonnet):
- Input: ~65 × 60k tokens = ~4M tokens → ~$12
- Output: ~65 × 15k tokens = ~1M tokens → ~$15
- Tool calls: ~3-5 per step, minimal additional cost
- Synthesis (Opus): ~1 call, ~$2-3
- **Total: ~$30-35 per full simulation run**

Acceptable for serious exploration. A sweep of 20 scenarios costs ~$600-700.

---

## Implementation Phases

### Phase 1: Oil Sub-Model (Proof of Concept)

Build just the fuel/oil system. ~20 variables. Historical forcing for everything else (if fuel model needs "bombing damage" as input, feed it from historical data rather than computing it).

- Implement the monthly step loop for fuel only
- Calibrate against USSBS fuel production data
- Verify: does the historical run reproduce the 1944 fuel crisis timeline?
- Test counterfactual: "2x synthetic fuel investment from 1940"

This validates the architecture (LLM reasoning + code computation + tool calls + validation) on a tractable sub-problem before scaling.

### Phase 2: Production + Bombing

Add industrial production categories and the strategic bombing interaction. This is the most data-rich part — USSBS provides monthly figures for most categories.

### Phase 3: Military Fronts

Add the military layer. This is where the most LLM judgment is needed (force ratios, operational outcomes). Start with the Eastern Front (best documented, most consequential).

### Phase 4: Full Integration

Connect all sub-models. Add remaining fronts, diplomacy, research programs. Run full historical validation.

### Phase 5: Counterfactual Campaign

Systematic exploration of decision space. Publish findings.

---

## Open Questions

1. **Allied response to counterfactuals.** If Germany doesn't declare war on the US, does the US still enter the European war? Probably (Lend-Lease escalation, Atlantic incidents), but the timeline and scale change. The "exogenous forcing function" assumption breaks for large counterfactuals. May need the LLM to reason about Allied reactions for certain scenarios.

2. **Operational-level resolution.** Monthly steps may be too coarse for key battles (Stalingrad unfolds over ~5 months, Kursk over 2 weeks). Consider variable time steps: weekly during critical periods, monthly otherwise.

3. **LLM historical accuracy.** How granular is the LLM's knowledge of month-by-month WW2 military-industrial data? Likely good for major events, weaker for specific production figures. The tool calls and hard caps mitigate this, but the reasoning quality still depends on the model knowing, e.g., that the Schweinfurt raids specifically targeted ball bearing production.

4. **Determinism vs. stochasticity.** Same inputs + same LLM call may produce slightly different outputs (temperature > 0). Running multiple iterations per scenario and averaging may be necessary. Alternatively, temperature=0 for reproducibility, accepting that the model picks one plausible path rather than sampling the distribution.

5. **Scope of "winning."** What counts as Germany "winning"? Unconditional Allied surrender is fantasy. Negotiated peace preserving some territorial gains? Stalemate with 1939 borders? Surviving as a state without occupation? The definition affects which counterfactuals are even worth testing. A reasonable target: "Germany avoids unconditional surrender and reaches a negotiated ceasefire by end of 1945."
