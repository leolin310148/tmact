package main

const workflowGenericExampleYAML = `version: 2
workspace:
  root: .
variables:
  greeting:
    type: string
    default: hello
revisions:
  source:
    git: {}
defaults:
  max_parallel: 1
  timeout: 10m
  retry:
    max_attempts: 2
    backoff: 5s
stages:
  - id: test
    type: command
    argv: ["go", "test", "./..."]
    inherit_env: [PATH, HOME]
    bind_revisions: [source]
  - id: approval
    type: human
    needs: [test]
    outcomes:
      approve: success
      reject: failed
`

// workflowOpenSpecProfileYAML is data consumed by the generic v2 engine. Keep
// domain behavior here rather than adding profile-specific branches to it.
const workflowOpenSpecProfileYAML = `version: 2
workspace:
  root: .
agents_config: agents.yaml
variables:
  change:
    type: string
    required: true
actors:
  pm:
    agent: pm
  swe:
    agent: swe
  qa:
    agent: qa
  reviewer:
    agent: reviewer
revisions:
  spec:
    files:
      paths: ["openspec/changes/{{ .vars.change }}"]
  source:
    git: {}
defaults:
  max_parallel: 2
  timeout: 2h
  poll_interval: 2s
  retry:
    max_attempts: 3
    backoff: 10s
stages:
  - id: validate_spec
    type: command
    argv: ["openspec", "validate", "{{ .vars.change }}", "--strict"]
    inherit_env: [PATH, HOME]
    bind_revisions: [spec]

  - id: pm_review
    type: agent
    needs: [validate_spec]
    actor: pm
    bind_revisions: [spec]
    prompt: Review the OpenSpec change {{ .vars.change }} for product correctness.
    outcomes: {accept: success, request_changes: retry, blocked: blocked}

  - id: swe_review
    type: agent
    needs: [pm_review]
    actor: swe
    bind_revisions: [spec]
    prompt: Review the OpenSpec change {{ .vars.change }} for implementation feasibility.
    outcomes: {accept: success, request_changes: retry, blocked: blocked}

  - id: qa_review
    type: agent
    needs: [swe_review]
    actor: qa
    bind_revisions: [spec]
    prompt: Review the OpenSpec change {{ .vars.change }} for testability and acceptance criteria.
    outcomes: {accept: success, request_changes: retry, blocked: blocked}

  - id: final_review
    type: agent
    needs: [qa_review]
    actor: reviewer
    bind_revisions: [spec]
    prompt: Perform the final independent review of OpenSpec change {{ .vars.change }}.
    outcomes: {accept: success, request_changes: retry, blocked: blocked}

  - id: apply
    type: agent
    needs: [final_review]
    actor: swe
    bind_revisions: [spec, source]
    produces_revisions: [source]
    prompt: Implement OpenSpec change {{ .vars.change }} completely. Do not archive it.
    outcomes: {complete: success, request_changes: retry, blocked: blocked}

  - id: validate_implementation
    type: command
    needs: [apply]
    argv: ["openspec", "validate", "{{ .vars.change }}", "--strict"]
    inherit_env: [PATH, HOME]
    bind_revisions: [spec, source]

  - id: test_implementation
    type: command
    needs: [apply]
    argv: ["go", "test", "./..."]
    inherit_env: [PATH, HOME, GOCACHE]
    bind_revisions: [spec, source]

  - id: qa_verify
    type: agent
    needs: [validate_implementation, test_implementation]
    actor: qa
    bind_revisions: [spec, source]
    prompt: Semantically verify change {{ .vars.change }} using the saved machine evidence.
    outcomes: {pass: success, fail: retry, blocked: blocked}

  - id: archive_gate
    type: gate
    needs: [qa_verify]
    condition:
      all:
        - revision: {name: spec, stage: final_review}
        - revision: {name: source, stage: qa_verify}

  - id: archive
    type: agent
    needs: [archive_gate]
    actor: pm
    bind_revisions: [spec, source]
    produces_revisions: [spec]
    prompt: Archive OpenSpec change {{ .vars.change }} now that all gates pass.
    outcomes: {complete: success, blocked: blocked}
`
