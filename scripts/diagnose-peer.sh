#!/usr/bin/env bash
#
# Probe one statusd peer for a while and write per-sample diagnostics as JSONL.
#
# The useful distinction is:
#   - peer-a host ping OK, WSL peer HTTP timeout: host/overlay network is reachable,
#     but the WSL side or forwarding into WSL is slow/unreachable.
#   - /api/health OK, /api/snapshot timeout: the peer daemon has a cached
#     snapshot, but serving/encoding the full snapshot is stuck.
#   - /api/version OK, /api/snapshot timeout: peer HTTP is reachable, but the
#     remote statusd snapshot path is slow/stuck (often tmux/WSL/load).
#   - /api/version and /api/snapshot both timeout: network, Tailscale path,
#     remote HTTP server, or remote machine availability.
#
# Usage:
#   scripts/diagnose-peer.sh --duration 30m
#   scripts/diagnose-peer.sh --host peer-a --duration 30m
#   scripts/diagnose-peer.sh --peer peer-a --url http://peer-a.example:7890 --duration 10m
#
# Output:
#   /tmp/tmact-peer-diagnose-<peer>-<timestamp>.jsonl
#   /tmp/tmact-peer-diagnose-<peer>-<timestamp>.summary.json

set -euo pipefail

CONFIG_PATH="${TMACT_STATUSD_CONFIG:-$HOME/.tmact/statusd.json}"
LOCAL_URL="${TMACT_LOCAL_STATUSD_URL:-http://127.0.0.1:7890}"
PEER_NAME=""
PEER_URL=""
HOST_NAME="${TMACT_PEER_HOST:-}"
HOST_URL="${TMACT_PEER_HOST_URL:-}"
DURATION="10m"
INTERVAL="1s"
TIMEOUT="2s"
CONNECT_TIMEOUT="1s"
PING_EVERY="30s"
OUT=""

usage() {
  sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

die() {
  echo "error: $*" >&2
  exit 2
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

duration_to_seconds() {
  local raw="$1"
  case "$raw" in
    *ms) die "duration in milliseconds is not supported here: $raw" ;;
    *s) echo "${raw%s}" ;;
    *m) echo "$(( ${raw%m} * 60 ))" ;;
    *h) echo "$(( ${raw%h} * 3600 ))" ;;
    ''|*[!0-9]*) die "invalid duration: $raw" ;;
    *) echo "$raw" ;;
  esac
}

duration_to_curl_seconds() {
  local raw="$1"
  case "$raw" in
    *ms) awk "BEGIN { printf \"%.3f\", ${raw%ms} / 1000 }" ;;
    *s) echo "${raw%s}" ;;
    *m) echo "$(( ${raw%m} * 60 ))" ;;
    *h) echo "$(( ${raw%h} * 3600 ))" ;;
    ''|*[!0-9]*) die "invalid duration: $raw" ;;
    *) echo "$raw" ;;
  esac
}

json_bool() {
  if [[ "$1" == "1" ]]; then
    echo true
  else
    echo false
  fi
}

host_from_url() {
  local url="$1"
  url="${url#http://}"
  url="${url#https://}"
  url="${url%%/*}"
  url="${url%%:*}"
  printf '%s\n' "$url"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config) CONFIG_PATH="${2:-}"; shift 2 ;;
      --local-url) LOCAL_URL="${2:-}"; shift 2 ;;
      --peer|--peer-name) PEER_NAME="${2:-}"; shift 2 ;;
      --url|--peer-url) PEER_URL="${2:-}"; shift 2 ;;
      --host|--host-name) HOST_NAME="${2:-}"; shift 2 ;;
      --host-url) HOST_URL="${2:-}"; shift 2 ;;
      --duration) DURATION="${2:-}"; shift 2 ;;
      --interval) INTERVAL="${2:-}"; shift 2 ;;
      --timeout) TIMEOUT="${2:-}"; shift 2 ;;
      --connect-timeout) CONNECT_TIMEOUT="${2:-}"; shift 2 ;;
      --ping-every) PING_EVERY="${2:-}"; shift 2 ;;
      --out) OUT="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown argument: $1" ;;
    esac
  done
}

load_peer_from_config() {
  if [[ -n "$PEER_NAME" && -n "$PEER_URL" ]]; then
    return
  fi
  if [[ ! -f "$CONFIG_PATH" ]]; then
    return
  fi
  if [[ -z "$PEER_NAME" ]]; then
    PEER_NAME="$(jq -r '.peers[0].name // empty' "$CONFIG_PATH")"
  fi
  if [[ -z "$PEER_URL" ]]; then
    if [[ -n "$PEER_NAME" ]]; then
      PEER_URL="$(jq -r --arg name "$PEER_NAME" '.peers[]? | select(.name == $name) | .url' "$CONFIG_PATH" | head -n 1)"
    else
      PEER_URL="$(jq -r '.peers[0].url // empty' "$CONFIG_PATH")"
    fi
  fi
}

snapshot_meta_json() {
  local body="$1"
  if jq -e . "$body" >/dev/null 2>&1; then
    jq -c '{
      snapshot_ts: (.ts // null),
      generated_by: (.generated_by // null),
      sessions: (.summary.sessions // null),
      panes: (.summary.panes // null),
      working: (.summary.working // null),
      asking: (.summary.asking // null),
      summary_errors: (.summary.errors // null),
      error_scopes: ([.errors[]?.scope] | unique)
    }' "$body"
  else
    printf '{}\n'
  fi
}

health_meta_json() {
  local body="$1"
  if jq -e . "$body" >/dev/null 2>&1; then
    jq -c '{
      health_ok: (.ok // null),
      health_snapshot_available: (.snapshot_available // null),
      health_snapshot_ts: (.snapshot_ts // null),
      health_snapshot_age_ms: (.snapshot_age_ms // null),
      health_interval_ms: (.interval_ms // null),
      health_stale_after_ms: (.stale_after_ms // null),
      health_sessions: (.summary.sessions // null),
      health_panes: (.summary.panes // null),
      health_working: (.summary.working // null),
      health_asking: (.summary.asking // null),
      health_error_count: (.error_count // null)
    }' "$body"
  else
    printf '{}\n'
  fi
}

local_peer_meta_json() {
  local body="$1"
  local peer="$2"
  if jq -e . "$body" >/dev/null 2>&1; then
    jq -c --arg peer "$peer" '{
      snapshot_ts: (.ts // null),
      peer_panes: ([.panes[]? | select(.peer == $peer)] | length),
      peer_stale_panes: ([.panes[]? | select(.peer == $peer and .stale == true)] | length),
      peer_sessions: ([.sessions[]? | select(.peer == $peer)] | length),
      peer_stale_sessions: ([.sessions[]? | select(.peer == $peer and .stale == true)] | length),
      peer_errors: ([.errors[]? | select(.scope == ("peer:" + $peer)) | .error])
    }' "$body"
  else
    printf '{}\n'
  fi
}

curl_probe_json() {
  local label="$1"
  local url="$2"
  local meta_kind="${3:-none}"
  local body err write rc ok code time_connect time_starttransfer time_total size_download remote_ip local_ip curl_exit err_msg stderr_text meta
  body="$(mktemp "${TMPDIR:-/tmp}/tmact-peer-body.XXXXXX")"
  err="$(mktemp "${TMPDIR:-/tmp}/tmact-peer-curl.XXXXXX")"

  set +e
  write="$(curl -sS -o "$body" \
    --connect-timeout "$CURL_CONNECT_TIMEOUT" \
    --max-time "$CURL_TIMEOUT" \
    -w $'%{http_code}\t%{time_connect}\t%{time_starttransfer}\t%{time_total}\t%{size_download}\t%{remote_ip}\t%{local_ip}\t%{exitcode}\t%{errormsg}' \
    "$url" 2>"$err")"
  rc=$?
  set -e

  IFS=$'\t' read -r code time_connect time_starttransfer time_total size_download remote_ip local_ip curl_exit err_msg <<< "$write"
  code="${code:-0}"
  curl_exit="${curl_exit:-$rc}"
  stderr_text="$(tr '\n' ' ' < "$err" | sed 's/[[:space:]]*$//')"
  ok=0
  if [[ "$rc" -eq 0 && "$code" =~ ^2 ]]; then
    ok=1
  fi

  case "$meta_kind" in
    snapshot) meta="$(snapshot_meta_json "$body")" ;;
    health) meta="$(health_meta_json "$body")" ;;
    local) meta="$(local_peer_meta_json "$body" "$PEER_NAME")" ;;
    *) meta="{}" ;;
  esac

  jq -cn \
    --arg label "$label" \
    --arg url "$url" \
    --arg code "$code" \
    --arg rc "$rc" \
    --arg curl_exit "$curl_exit" \
    --arg time_connect "${time_connect:-0}" \
    --arg time_starttransfer "${time_starttransfer:-0}" \
    --arg time_total "${time_total:-0}" \
    --arg size_download "${size_download:-0}" \
    --arg remote_ip "${remote_ip:-}" \
    --arg local_ip "${local_ip:-}" \
    --arg err_msg "${err_msg:-}" \
    --arg stderr "$stderr_text" \
    --argjson ok "$(json_bool "$ok")" \
    --argjson meta "$meta" '
      {
        label: $label,
        url: $url,
        ok: $ok,
        http_code: ($code | tonumber? // 0),
        curl_rc: ($rc | tonumber? // 0),
        curl_exit: ($curl_exit | tonumber? // 0),
        connect_ms: (($time_connect | tonumber? // 0) * 1000),
        ttfb_ms: (($time_starttransfer | tonumber? // 0) * 1000),
        total_ms: (($time_total | tonumber? // 0) * 1000),
        bytes: ($size_download | tonumber? // 0),
        remote_ip: $remote_ip,
        local_ip: $local_ip,
        error: (if $err_msg != "" then $err_msg elif $stderr != "" then $stderr else null end)
      } + $meta'

  rm -f "$body" "$err"
}

ping_probe_json() {
  local host="$1"
  local wait_arg output rc ok
  if ! command -v ping >/dev/null 2>&1; then
    jq -cn '{ran:false, ok:null, reason:"ping command not found"}'
    return
  fi
  case "$(uname)" in
    Darwin) wait_arg=( -W 1000 ) ;;
    *) wait_arg=( -W 1 ) ;;
  esac
  set +e
  output="$(ping -c 1 "${wait_arg[@]}" "$host" 2>&1)"
  rc=$?
  set -e
  ok=0
  if [[ "$rc" -eq 0 ]]; then
    ok=1
  fi
  jq -cn --arg host "$host" --arg output "$output" --arg rc "$rc" --argjson ok "$(json_bool "$ok")" \
    '{ran:true, host:$host, ok:$ok, rc:($rc | tonumber), output:$output}'
}

tailscale_ping_json() {
  local host="$1"
  local output rc ok
  if ! command -v tailscale >/dev/null 2>&1; then
    jq -cn '{ran:false, ok:null, reason:"tailscale command not found"}'
    return
  fi
  set +e
  output="$(tailscale ping --timeout 2s --c 1 "$host" 2>&1)"
  rc=$?
  set -e
  ok=0
  if [[ "$rc" -eq 0 ]]; then
    ok=1
  fi
  jq -cn --arg host "$host" --arg output "$output" --arg rc "$rc" --argjson ok "$(json_bool "$ok")" \
    '{ran:true, host:$host, ok:$ok, rc:($rc | tonumber), output:$output}'
}

host_probe_json() {
  local host="$1"
  local host_url="$2"
  local ping_json ts_json http_json
  if [[ -n "$host" ]]; then
    ping_json="$(ping_probe_json "$host")"
    ts_json="$(tailscale_ping_json "$host")"
  else
    ping_json="$(jq -cn '{ran:false, reason:"host not configured"}')"
    ts_json="$(jq -cn '{ran:false, reason:"host not configured"}')"
  fi
  if [[ -n "$host_url" ]]; then
    host_url="${host_url%/}"
    http_json="$(curl_probe_json host_version "$host_url/api/version")"
  else
    http_json="$(jq -cn '{label:"host_version", ok:null, reason:"host URL not configured"}')"
  fi
  jq -cn \
    --arg name "$host" \
    --arg url "$host_url" \
    --argjson ping "$ping_json" \
    --argjson tailscale_ping "$ts_json" \
    --argjson http "$http_json" \
    '{name:$name, url:$url, ping:$ping, tailscale_ping:$tailscale_ping, http:$http}'
}

write_summary() {
  local out="$1"
  local summary_path="${out%.jsonl}.summary.json"
  jq -s '
    def count_where(f): map(select(f)) | length;
    {
      peer: (.[0].peer // null),
      url: (.[0].url // null),
      first_ts: (.[0].ts // null),
      last_ts: (.[-1].ts // null),
      samples: length,
      snapshot_failures: count_where(.snapshot.ok == false),
      version_failures: count_where(.version.ok == false),
      health_failures: count_where(.health.ok == false),
      local_failures: count_where(.local.ok == false),
      local_peer_stale_samples: count_where((.local.peer_stale_panes // 0) > 0 or (.local.peer_stale_sessions // 0) > 0),
      snapshot_failed_but_health_ok: count_where(.snapshot.ok == false and .health.ok == true),
      snapshot_failed_but_version_ok: count_where(.snapshot.ok == false and .version.ok == true),
      snapshot_and_version_both_failed: count_where(.snapshot.ok == false and .version.ok == false),
      ping_failures_when_ran: count_where(.ping.ran == true and .ping.ok == false),
      host_ping_failures_when_ran: count_where(.host.ping.ran == true and .host.ping.ok == false),
      host_tailscale_ping_failures_when_ran: count_where(.host.tailscale_ping.ran == true and .host.tailscale_ping.ok == false),
      host_http_failures_when_configured: count_where(.host.http.ok == false),
      wsl_failed_host_ping_ok: count_where(.snapshot.ok == false and .host.ping.ok == true),
      wsl_failed_host_tailscale_ok: count_where(.snapshot.ok == false and .host.tailscale_ping.ok == true),
      avg_snapshot_total_ms: ((map(select(.snapshot.ok == true) | .snapshot.total_ms) | add) / (map(select(.snapshot.ok == true)) | length) // null),
      max_snapshot_total_ms: (map(.snapshot.total_ms) | max // null),
      recent_snapshot_errors: ([.[] | select(.snapshot.ok == false) | {ts, health_ok: .health.ok, health_snapshot_age_ms: .health.health_snapshot_age_ms, error: .snapshot.error}] | .[-10:]),
      recent_host_ping_errors: ([.[] | select(.host.ping.ran == true and .host.ping.ok == false) | {ts, output: .host.ping.output}] | .[-10:]),
      recent_host_tailscale_ping_errors: ([.[] | select(.host.tailscale_ping.ran == true and .host.tailscale_ping.ok == false) | {ts, output: .host.tailscale_ping.output}] | .[-10:]),
      recent_local_peer_errors: ([.[] | select((.local.peer_errors // []) | length > 0) | {ts, errors: .local.peer_errors}] | .[-10:])
    }' "$out" > "$summary_path"

  echo "summary: $summary_path" >&2
  jq -r '
    "samples=\(.samples) snapshot_failures=\(.snapshot_failures) health_failures=\(.health_failures) version_failures=\(.version_failures) local_peer_stale_samples=\(.local_peer_stale_samples)",
    "snapshot_failed_but_health_ok=\(.snapshot_failed_but_health_ok) snapshot_failed_but_version_ok=\(.snapshot_failed_but_version_ok) snapshot_and_version_both_failed=\(.snapshot_and_version_both_failed) ping_failures_when_ran=\(.ping_failures_when_ran)",
    "host_ping_failures_when_ran=\(.host_ping_failures_when_ran) host_tailscale_ping_failures_when_ran=\(.host_tailscale_ping_failures_when_ran) wsl_failed_host_ping_ok=\(.wsl_failed_host_ping_ok) wsl_failed_host_tailscale_ok=\(.wsl_failed_host_tailscale_ok)",
    "avg_snapshot_total_ms=\(.avg_snapshot_total_ms // "n/a") max_snapshot_total_ms=\(.max_snapshot_total_ms // "n/a")"
  ' "$summary_path" >&2
}

main() {
  parse_args "$@"
  require_cmd curl
  require_cmd jq

  load_peer_from_config
  [[ -n "$PEER_NAME" ]] || die "peer name is required; pass --peer or configure peers in $CONFIG_PATH"
  [[ -n "$PEER_URL" ]] || die "peer URL is required; pass --url or configure peers in $CONFIG_PATH"

  PEER_URL="${PEER_URL%/}"
  LOCAL_URL="${LOCAL_URL%/}"
  CURL_TIMEOUT="$(duration_to_curl_seconds "$TIMEOUT")"
  CURL_CONNECT_TIMEOUT="$(duration_to_curl_seconds "$CONNECT_TIMEOUT")"
  local duration_s interval_s ping_every_s start_s end_s next_ping_s now_s wsl_host sample ping_json host_json snapshot_json health_json version_json local_json line
  duration_s="$(duration_to_seconds "$DURATION")"
  interval_s="$(duration_to_seconds "$INTERVAL")"
  ping_every_s="$(duration_to_seconds "$PING_EVERY")"
  [[ "$duration_s" -gt 0 ]] || die "--duration must be > 0"
  [[ "$interval_s" -gt 0 ]] || die "--interval must be > 0"
  [[ "$ping_every_s" -gt 0 ]] || die "--ping-every must be > 0"

  if [[ -z "$OUT" ]]; then
    OUT="/tmp/tmact-peer-diagnose-${PEER_NAME}-$(date +%Y%m%d-%H%M%S).jsonl"
  fi
  mkdir -p "$(dirname "$OUT")"
  : > "$OUT"

  wsl_host="$(host_from_url "$PEER_URL")"
  start_s="$(date +%s)"
  end_s=$(( start_s + duration_s ))
  next_ping_s="$start_s"

  echo "peer=$PEER_NAME url=$PEER_URL local=$LOCAL_URL host=${HOST_NAME:-"(not configured)"} host_url=${HOST_URL:-"(not configured)"} duration=${duration_s}s interval=${interval_s}s timeout=${TIMEOUT}" >&2
  echo "writing: $OUT" >&2

  while :; do
    now_s="$(date +%s)"
    if [[ "$now_s" -ge "$end_s" ]]; then
      break
    fi

    sample="$(date +%Y-%m-%dT%H:%M:%S%z)"
    version_json="$(curl_probe_json version "$PEER_URL/api/version")"
    health_json="$(curl_probe_json health "$PEER_URL/api/health" health)"
    snapshot_json="$(curl_probe_json snapshot "$PEER_URL/api/snapshot" snapshot)"
    local_json="$(curl_probe_json local "$LOCAL_URL/api/snapshot" local)"

    if [[ "$now_s" -ge "$next_ping_s" ]] || jq -e '.ok == false' >/dev/null <<< "$snapshot_json"; then
      ping_json="$(ping_probe_json "$wsl_host")"
      host_json="$(host_probe_json "$HOST_NAME" "$HOST_URL")"
      next_ping_s=$(( now_s + ping_every_s ))
    else
      ping_json="$(jq -cn '{ran:false}')"
      host_json="$(jq -cn \
        --arg name "$HOST_NAME" \
        --arg url "$HOST_URL" \
        '{name:$name, url:$url, ping:{ran:false}, tailscale_ping:{ran:false}, http:{label:"host_version", ok:null, ran:false}}')"
    fi

    line="$(jq -cn \
      --arg ts "$sample" \
      --arg peer "$PEER_NAME" \
      --arg url "$PEER_URL" \
      --argjson version "$version_json" \
      --argjson health "$health_json" \
      --argjson snapshot "$snapshot_json" \
      --argjson local "$local_json" \
      --argjson ping "$ping_json" \
      --argjson host "$host_json" \
      '{ts:$ts, peer:$peer, url:$url, version:$version, health:$health, snapshot:$snapshot, local:$local, ping:$ping, host:$host}')"
    printf '%s\n' "$line" | tee -a "$OUT" >/dev/null

    if jq -e '.snapshot.ok == false or .health.ok == false or .version.ok == false or ((.local.peer_errors // []) | length > 0) or (.host.ping.ok == false) or (.host.tailscale_ping.ok == false)' >/dev/null <<< "$line"; then
      jq -r '"\(.ts) snapshot_ok=\(.snapshot.ok) health_ok=\(.health.ok) health_age_ms=\(.health.health_snapshot_age_ms // "n/a") version_ok=\(.version.ok) wsl_ping=\(.ping.ok // "n/a") host_ping=\(.host.ping.ok // "n/a") host_ts=\(.host.tailscale_ping.ok // "n/a") local_peer_errors=\((.local.peer_errors // []) | join("; ")) snapshot_error=\(.snapshot.error // "")"' <<< "$line" >&2
    fi

    sleep "$interval_s"
  done

  write_summary "$OUT"
}

main "$@"
