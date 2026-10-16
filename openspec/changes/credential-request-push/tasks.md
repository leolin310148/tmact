# Tasks: Credential Request Push

## 1. Proposal Review

- [x] User reviews proposal, design, and spec.

## 2. Credential Store

- [x] Add `internal/credstore` with name validation, atomic 0600 write, list,
      remove, path, and owner/symlink checks.
- [x] Unit tests for invalid names, symlinked root/file, permissions.

## 3. statusd Requests

- [x] In-memory request registry with timeout and one-shot transitions.
- [x] Socket-only create and wait routes; metadata, submit, decline routes.
- [x] `Snapshot.CredRequests` and peer-prefixed merge.
- [x] Tests: TCP create rejected, double submit 409, expiry, no value in
      snapshot/metadata/logs.

## 4. Push and Peer Forwarding

- [x] Notifier that pushes once per request id when VAPID and subscriptions
      exist.
- [x] Forward metadata/submit/decline with `?peer=`; never proxy socket-only
      routes.
- [x] Tests with a fake push client and a fake peer.

## 5. CLI

- [ ] `tmact cred request|path|ls|rm|run` in `cmd/tmact/cred_cmd.go`, wired in
      `main.go` and the help catalog.
- [ ] Tests for exit codes, `--refresh`, and `cred run` env injection.

## 6. Web UI

- [ ] `?cred=` boot param and `OPEN_CRED` service-worker message.
- [ ] `CredentialDialog` and pending-request badge; top-level window only.

## 7. Skill and Docs

- [ ] `skills/tmact-cred/SKILL.md` + `agents/openai.yaml`; run
      `quick_validate.py` and `scripts/install-skills.sh --check`.
- [ ] README section.

## 8. Live Check

- [ ] Request from a studio pane and from a mini pane, submit from the phone,
      confirm one notification each and that `cred run` works.
