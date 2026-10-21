# Agent routing boundary

`modelroute` owns model and effort decision policy shared by dispatch
and ask. The CLI enables it when decide is installed and no model is specified;
explicit models bypass it, while explicit providers and effort constrain it.
Library/API callers opt in through options. It depends on quota data types,
not tmux or the web server. `dispatch`
owns lifecycle safety and the launcher model/effort allowlists. No model
response can become a shell fragment: decide returns an opaque model ID and that model's effort choice; tmact maps
them to a maintained, validated launch tuple.

Provider quota is read by tmact. Only task text, current time, sanitized quota,
and candidate descriptions cross the decide process/API boundary. Account
identifiers, raw provider failures, credentials and ask reply capability IDs
stay outside that input. Authentication is inherited by decide; tmact neither
reads its secret config values nor relays them to a peer. Repository-local
decide configuration is ignored and full decide usage logging is disabled.

A decision is bounded and validated against the eligible profiles. Confidence
is reported for reference and never blocks a valid selection. Failure stops
before closing or launching an agent. Provider errors
exclude that provider rather than hiding the working one. Codex has only a
weekly limit; Claude additionally needs a five-hour reserve. Relative weekly
headroom and the task's required capability guide selection. Model tiers are coarse descriptions, not equivalence claims or a fixed
difficulty-to-model/effort mapping.

Routing applies to fresh work. An existing idle Claude/Codex agent is exited
only after selection succeeds, and the same pane must return to a shell before
launch. Busy agents, drafts, permission prompts and exit timeouts are blockers;
there is no kill fallback. Continuing conversations (`--no-clear`, queue, ask
threads) preserve the agent and make no routing request.

Ask preflights selection before creating a reply capability, persists the
selected agent in its request record, then rechecks lifecycle guards on actual
dispatch. Peer dispatch uses a read-only protocol probe to obtain target quota
and routing candidates; decide runs on the caller, then the peer revalidates
launch settings and lifecycle. Protocol capability checks reject old peers
before any execution with new launch options.

## Task-file evidence and audit records

Only `.md`/`.txt` paths explicitly present in the original prompt are read as
routing context; file contents are separate untrusted task data, not policy.
There is no recursive traversal, shell expansion or HTTP fetch. Relative paths
use the actual pane cwd; a peer reads its own files before returning routing
context. The caller preserves those snapshots instead of reading same-named
local files. The delivered prompt is never expanded or rewritten.

File snapshots have bounded size/count and expose missing/truncated status.
Incomplete attachments do not become a new gate. No-clear, queue and thread
continuations make no routing or attachment request.

An independent difficulty question measures intrinsic work, not the provider
selected under quota constraints. Decision records retain exact input,
attachments/hash, policy version/hash, typed questions and response, selection
and elapsed time in a private append-only JSONL log. Authentication and appended
ask capability instructions never enter this record. A dispatch ID correlates
planned/executed dispatch and ask reply outcomes; it is distinct from the ask
capability. Older ask records without dispatch IDs remain readable.

The audit log is local and best-effort; inability to write it never prevents
work. The query interface omits private task/file content by default and needs
`--show-content` for evidence inspection. Delivery or an answerer reply is an
observation, not proof of correctness or a difficulty ground truth.

## Saved CLI plans

New ask and dispatch-work dry-runs automatically persist a private, versioned
plan in a temporary directory and expose its path plus an apply command.
The artifact freezes the original CLI task/options and selected launch tuple;
apply never calls decide or refreshes routing quota. Direct execution without
a plan remains available and performs a new decision. Plans add no expiry or
confidence threshold: quota movement alone does not invalidate reviewed work.

The CLI owns artifact parsing, digest validation and option replay; dispatch
owns live state validation and side-effect safety. A plan cannot accept task
or launch overrides. File fingerprints cover full referenced files, even beyond
routing excerpts; session existence, session/pane identity, runtime and cwd must
match. Peer checks execute on the peer under a versioned protocol. Existing
busy/prompt/trust/lease guards remain authoritative at apply. The digest catches
accidental edits, not hostile rewriting by the local owner; plan files must be
treated as private executable work instructions. The plan is not an authorization
capability and never contains an ask reply capability.

## Model catalog and effort decisions

The automatic catalog contains Claude Haiku 5.5, Sonnet 5.5, Opus 5.5,
Fable 5.1, and Codex GPT-6 Luna, GPT-6.1 Sol, GPT-6 Astra.
Each model has its own capability description and supported effort list.
Model-specific weekly caps filter models, independently of effort. The catalog
is maintained explicitly; it is not whatever a model can recall about new names.

A single decide request asks for the model, intrinsic difficulty, and the effort
appropriate if each eligible model were used. Only the effort answer for the
selected model is applied. Missing or unsupported selected effort is an invalid
answer; answers for unselected models are immaterial. Explicit effort pins that
value and removes the effort questions. Legacy pinned candidates and saved plans
remain readable. Explicit CLI model selection still bypasses auto routing.

Separate conditional effort questions avoid an invalid cross-model combination
without a second API call or a Cartesian expansion repeating model descriptions.
The state and descriptions are shared once; criteria reference candidate IDs.
Effort confidence is logged separately, with no acceptance threshold. Generic
worker rules and irrelevant examples must not alone cause a difficulty escalation.

Capability descriptions checked 2026-10-21:
- [OpenAI model catalog](https://developers.openai.com/api/docs/models) and
  model pages for [Luna](https://developers.openai.com/api/docs/models/gpt-6-luna),
  [Terra](https://developers.openai.com/api/docs/models/gpt-5.6-terra),
  [Sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol), and
  [Astra](https://developers.openai.com/api/docs/models/gpt-6-astra): Luna targets
  focused high-volume work; Sol targets complex work with near-Astra
  performance at lower cost than Astra; Astra targets the most demanding work.
  Terra (GPT-5.6) is no longer in the current catalog and is excluded from
  automatic routing: third-party results (Vals AI, Artificial Analysis) put Sol
  far ahead on agentic coding (Terminal-Bench 4.0 ~55% vs ~23%) and only a few
  points behind Astra (~58-60%). Sol/Astra descriptions cite that small margin.
- [Claude model comparison](https://platform.claude.com/docs/en/models/overview):
  Haiku targets high-volume, latency-sensitive work; Sonnet balances speed and
  intelligence; Opus targets long-running agentic coding and knowledge work;
  Fable targets demanding reasoning and long-horizon agentic work, including
  when Opus at higher effort falls short. Anthropic's general starting-point
  recommendation is capability context, not an override of tmact quota policy.
- [OpenAI API pricing](https://developers.openai.com/api/docs/pricing) does not
  establish Codex subscription quota consumption. Descriptions do not encode
  API dollar rates as subscription quota weights, assume equal capability at
  equal tiers, or target a model's selection share. Effort remains independent.

Launcher effort sources checked 2026-10-21:
- [Claude effort](https://platform.claude.com/docs/en/build-with-claude/effort)
  and [Fable 5.1](https://platform.claude.com/docs/en/models/fable-5-1/overview):
  all four maintained Claude models support low, medium, high, xhigh, max.
- Codex CLI 0.163.0 model metadata (`models_cache.json`) supplies launcher
  supported reasoning levels: Luna supports low through max;
  Sol and Astra also advertise ultra (automatic task delegation).
  The [Terra API reference](https://developers.openai.com/api/docs/models/gpt-5.6-terra)
  describes the direct API through max; tmact launches Codex CLI, whose ultra
  orchestration option is separate from the direct API effort contract.

Peer auto routing requires protocol version 5 so an old fixed-profile peer is
not silently used as the new catalog. Caller-owned descriptions and effort
capabilities are authoritative; intersect the peer's eligible efforts and caller
constraints rather than trusting peer-supplied routing instructions.

## Section evidence, bounded context, and safe failures

File identity includes its canonical path and section selector, not just path.
ATX headings include their children until the next same/higher heading; fenced
code is not an outline. Exact title and generated anchor matching must be
unambiguous. Missing/ambiguous selectors are explicit omitted evidence, never
whole-file fallback. Eight physical file slots are distinct from the 32 section
reference slots. Full-source hashes bind selected ranges to saved plans even
when the routing excerpt is shortened; changed sources or selectors need a new
plan. Old plans with older file metadata may require regeneration.

Source scanning is bounded at 8 MiB; collected excerpts remain bounded at
64 KiB each / 128 KiB total. Before crossing the decide boundary, budget the
entire serialized request and the largest single-question envelope with shared
state. The 48 KiB / 96 KiB UTF-8 heuristics include a 2 KiB model/framing reserve.
They are deliberately not called token counts or guaranteed tokenizer limits.
Only attachments are clipped, fairly across references; core task, policy,
quota and constraints are never silently changed. Metadata distinguishes source
size/hash, original selected range, included bytes/hash, and omission reason.

Allowlisted typed routing failures expose no upstream response text or secret
config data. Only a parsed HTTP max_tokens_exceeded error may halve attachment
content and retry once; it must actually change the input. Auth, unavailable,
invalid requests/responses and other errors stop. There is no implicit model
fallback. If the core request cannot fit without attachments, fail locally with
context_too_large before creating a reply capability or touching an agent.
One decision record retains all attempts, exact supplied snapshots, byte budget,
response and safe error code. Default queries redact per-attempt content too.
