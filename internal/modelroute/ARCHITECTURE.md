# Agent routing boundary

`modelroute` owns the task-to-profile decision policy shared by dispatch
and ask. The CLI enables it when decide is installed and no model is specified;
explicit models bypass it, while explicit providers and effort constrain it.
Library/API callers opt in through options. It depends on quota data types,
not tmux or the web server. `dispatch`
owns lifecycle safety and the launcher model/effort allowlists. No model
response can become a shell fragment: decide returns an opaque profile ID and
tmact maps it to a maintained, validated tuple.

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
headroom and the task's required capability guide selection. Profile tiers are
maintained routing assumptions, not benchmark claims.

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
