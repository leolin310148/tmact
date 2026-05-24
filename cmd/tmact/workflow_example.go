package main

const workflowExampleYAML = `change: your-change-id
agents_config: examples/openspec-workflow-agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
  reviewer: reviewer-agent
prompt_dispatch:
  clear_before_prompt: true
  clear_command: /clear
  clear_delay: 5s
  legacy_marker_fallback: false
discussion:
  role_order: [pm, swe, qa, reviewer]
  max_turns: 24
  max_runtime: 8h
  poll_interval: 15s
  idle_after: 30s
  capture_lines: 180
  create_missing_proposal: false
implementation:
  stage_order: [swe_apply, qa_verify, pm_archive]
  max_turns: 12
  max_runtime: 8h
  poll_interval: 15s
  idle_after: 30s
  capture_lines: 180
  require_phase1_agreed: true
  allow_dry_run_without_phase1: true
  apply_instructions:
    command: openspec
    args: ["instructions", "apply", "--change", "{{change}}"]
  verify_commands:
    - command: openspec
      args: ["validate", "{{change}}", "--strict"]
    - command: go
      args: ["test", "./..."]
  archive_command:
    command: openspec
    args: ["archive", "{{change}}", "--yes"]
log_path: .tmact/openspec-full-workflow.jsonl
`
