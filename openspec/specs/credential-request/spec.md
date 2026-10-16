# credential-request Specification

## Purpose
Define how an agent obtains a secret from the human through a web push and
uses it without the value entering agent context, pane text, logs, or any
HTTP response, including requests made on a federated peer.

## Requirements

### Requirement: Credential Request From An Agent

The system SHALL let a local process request a named credential from the human
and block until it is submitted, declined, or expired.

#### Scenario: Agent receives a path after submit

- GIVEN no file exists at `~/.tmact/.cred/github-pat`
- WHEN an agent runs `tmact cred request github-pat --reason "push tag"`
- AND the user submits a value in the web UI
- THEN the value is written to `~/.tmact/.cred/github-pat` with mode 0600
- AND the CLI prints only that path and exits 0

#### Scenario: Existing credential skips the push

- GIVEN `~/.tmact/.cred/github-pat` exists
- WHEN an agent runs `tmact cred request github-pat` without `--refresh`
- THEN no push is sent and the CLI prints the path

#### Scenario: Decline and timeout

- WHEN the user declines, the CLI exits 3
- WHEN the request expires, the CLI exits 4 and a later submit fails with 409

### Requirement: Create Is Local Only

The system SHALL accept credential request creation only over the statusd unix
socket.

#### Scenario: TCP create is rejected

- WHEN a client posts to `/api/cred/requests` over TCP
- THEN statusd responds 403 and no push is sent

### Requirement: Values Are Write-Only

The system SHALL NOT return a stored or submitted credential value from any
endpoint, snapshot, push payload, log line, or CLI output other than the
environment of a `tmact cred run` child process.

#### Scenario: Submitted value does not leak

- GIVEN a request was submitted with a marker value
- THEN the marker does not appear in `/api/snapshot`, request metadata,
  statusd logs, push payloads, or `cred request|path|ls` output

### Requirement: One-Shot Requests

The system SHALL accept at most one submit or decline per request id.

#### Scenario: Second submit fails

- GIVEN a request has been submitted
- WHEN another submit arrives for the same id
- THEN statusd responds 409 and the stored file is unchanged

### Requirement: Peer Requests

The system SHALL let the push hub surface and answer credential requests that
originate on a configured peer, storing the value only on the peer.

#### Scenario: Request from mini is answered on studio

- GIVEN studio has push subscriptions and mini is a configured peer
- WHEN an agent on mini requests a credential
- THEN studio sends one push linking to `/?cred=mini@<id>`
- AND a submit on studio is forwarded to mini, which writes the file
- AND studio's disk never holds the value
