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
    outcomes: {accept: success, request_changes: success, blocked: blocked}

  - id: pm_revision
    type: agent
    needs: [pm_review]
    when: {stage: {id: pm_review, outcome: request_changes}}
    actor: pm
    bind_revisions: [spec]
    produces_revisions: [spec]
    prompt: Revise OpenSpec change {{ .vars.change }} to address the PM review feedback in {{ .stages.pm_review.evidence.body }}.
    outcomes: {complete: success, blocked: blocked}

  - id: validate_pm_revision
    type: command
    needs: [pm_revision]
    when: {stage: {id: pm_review, outcome: request_changes}}
    argv: ["openspec", "validate", "{{ .vars.change }}", "--strict"]
    inherit_env: [PATH, HOME]
    bind_revisions: [spec]

  - id: swe_review
    type: agent
    needs: [validate_pm_revision]
    actor: swe
    bind_revisions: [spec]
    prompt: Review the OpenSpec change {{ .vars.change }} for implementation feasibility.
    outcomes: {accept: success, request_changes: success, blocked: blocked}

  - id: swe_revision
    type: agent
    needs: [swe_review]
    when: {stage: {id: swe_review, outcome: request_changes}}
    actor: swe
    bind_revisions: [spec]
    produces_revisions: [spec]
    prompt: Revise OpenSpec change {{ .vars.change }} to address the SWE review feedback in {{ .stages.swe_review.evidence.body }}.
    outcomes: {complete: success, blocked: blocked}

  - id: validate_swe_revision
    type: command
    needs: [swe_revision]
    when: {stage: {id: swe_review, outcome: request_changes}}
    argv: ["openspec", "validate", "{{ .vars.change }}", "--strict"]
    inherit_env: [PATH, HOME]
    bind_revisions: [spec]

  - id: qa_review
    type: agent
    needs: [validate_swe_revision]
    actor: qa
    bind_revisions: [spec]
    prompt: Review the OpenSpec change {{ .vars.change }} for testability and acceptance criteria.
    outcomes: {accept: success, request_changes: success, blocked: blocked}

  - id: qa_revision
    type: agent
    needs: [qa_review]
    when: {stage: {id: qa_review, outcome: request_changes}}
    actor: qa
    bind_revisions: [spec]
    produces_revisions: [spec]
    prompt: Revise OpenSpec change {{ .vars.change }} to address the QA review feedback in {{ .stages.qa_review.evidence.body }}.
    outcomes: {complete: success, blocked: blocked}

  - id: validate_qa_revision
    type: command
    needs: [qa_revision]
    when: {stage: {id: qa_review, outcome: request_changes}}
    argv: ["openspec", "validate", "{{ .vars.change }}", "--strict"]
    inherit_env: [PATH, HOME]
    bind_revisions: [spec]

  - id: final_review
    type: agent
    needs: [validate_qa_revision]
    actor: reviewer
    bind_revisions: [spec]
    prompt: Perform the final independent review of OpenSpec change {{ .vars.change }}.
    outcomes: {accept: success, request_changes: success, blocked: blocked}

  - id: final_revision
    type: agent
    needs: [final_review]
    when: {stage: {id: final_review, outcome: request_changes}}
    actor: swe
    bind_revisions: [spec]
    produces_revisions: [spec]
    prompt: Revise OpenSpec change {{ .vars.change }} to address the final review feedback in {{ .stages.final_review.evidence.body }}.
    outcomes: {complete: success, blocked: blocked}

  - id: validate_final_revision
    type: command
    needs: [final_revision]
    when: {stage: {id: final_review, outcome: request_changes}}
    argv: ["openspec", "validate", "{{ .vars.change }}", "--strict"]
    inherit_env: [PATH, HOME]
    bind_revisions: [spec]

  - id: final_confirmation
    type: agent
    needs: [validate_final_revision]
    actor: reviewer
    bind_revisions: [spec]
    prompt: Independently confirm that OpenSpec change {{ .vars.change }} is ready for implementation. If issues remain, report request_changes so the workflow blocks for operator attention instead of retrying unchanged input.
    outcomes: {accept: success, request_changes: blocked, blocked: blocked}

  - id: apply
    type: agent
    needs: [final_confirmation]
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
    outcomes: {pass: success, fail: success, blocked: blocked}

  - id: repair_implementation
    type: agent
    needs: [qa_verify]
    when: {stage: {id: qa_verify, outcome: fail}}
    actor: swe
    bind_revisions: [spec, source]
    produces_revisions: [source]
    prompt: Repair the implementation of OpenSpec change {{ .vars.change }} using the QA feedback in {{ .stages.qa_verify.evidence.body }}.
    outcomes: {complete: success, blocked: blocked}

  - id: validate_repair
    type: command
    needs: [repair_implementation]
    when: {stage: {id: qa_verify, outcome: fail}}
    argv: ["openspec", "validate", "{{ .vars.change }}", "--strict"]
    inherit_env: [PATH, HOME]
    bind_revisions: [spec, source]

  - id: test_repair
    type: command
    needs: [repair_implementation]
    when: {stage: {id: qa_verify, outcome: fail}}
    argv: ["go", "test", "./..."]
    inherit_env: [PATH, HOME, GOCACHE]
    bind_revisions: [spec, source]

  - id: qa_confirmation
    type: agent
    needs: [validate_repair, test_repair]
    actor: qa
    bind_revisions: [spec, source]
    prompt: Independently confirm the implementation of OpenSpec change {{ .vars.change }} using the saved machine evidence. If issues remain, report fail so the workflow blocks for operator attention instead of retrying unchanged input.
    outcomes: {pass: success, fail: blocked, blocked: blocked}

  - id: archive_gate
    type: gate
    needs: [qa_confirmation]
    condition:
      all:
        - revision: {name: spec, stage: final_confirmation}
        - revision: {name: source, stage: qa_confirmation}

  - id: archive
    type: agent
    needs: [archive_gate]
    actor: pm
    bind_revisions: [spec, source]
    produces_revisions: [spec, source]
    prompt: Archive OpenSpec change {{ .vars.change }} now that all gates pass.
    outcomes: {complete: success, blocked: blocked}
`
