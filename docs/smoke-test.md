# Smoke Test Notes

Use this file for reproducible checks that are safe to share. Keep live pane
names, private repository paths, and personal machine details out of committed
notes.

## Suggested Checks

Build the local binary:

```sh
go build -o .cache/tmact ./cmd/tmact
```

Run unit tests:

```sh
go test ./...
```

List panes and refresh the target cache:

```sh
.cache/tmact ls
```

Run dry-run config checks:

```sh
.cache/tmact loop run --config examples/night-loop.yaml --dry-run --once
.cache/tmact loop run --config examples/maintenance-loop.yaml --dry-run --once --assume-idle-on-start
.cache/tmact watch --config examples/accept-question-watch.yaml --dry-run --once
.cache/tmact workflow example --profile openspec > tmact-openspec-workflow.yaml
.cache/tmact workflow validate --config tmact-openspec-workflow.yaml --var change=demo
.cache/tmact workflow plan --config tmact-openspec-workflow.yaml --var change=demo
```

## Shell hook events (live socket)

Start an isolated statusd (unix socket only — `--web-addr ""` avoids
colliding with an installed statusd's TCP port), then emit against a real
pane id from `tmact ls`:

```sh
.cache/tmact statusd start --web-addr "" --no-tmux-options \
  --pane-cols 0 --pane-rows 0 --socket-path /tmp/tmact-hooktest.sock &
.cache/tmact hook emit --type preexec --pane-id %5 --command-id s1 \
  --command "sleep 99" --socket-path /tmp/tmact-hooktest.sock
.cache/tmact statusd read --socket-path /tmp/tmact-hooktest.sock --json
# expect the pane working/running with signals shell_hook, shell_hook_active
.cache/tmact hook emit --type precmd --pane-id %5 --command-id s1 \
  --exit-code 0 --socket-path /tmp/tmact-hooktest.sock
.cache/tmact statusd read --socket-path /tmp/tmact-hooktest.sock --json
# expect the pane idle/input_ready with signals shell_hook, shell_hook_completed
```

Then diagnose the same socket with the read-only observability commands:

```sh
.cache/tmact hook state --socket-path /tmp/tmact-hooktest.sock
# expect pane %5 listed with its completed command (exit=0 matched)
.cache/tmact hook doctor --socket-path /tmp/tmact-hooktest.sock --pane-id %5
# expect tmux/socket/daemon/pane_events all [ok]; exit 0
.cache/tmact hook doctor --socket-path /tmp/does-not-exist.sock; echo $?
# expect socket + daemon [!!] and a non-zero exit
```

Also syntax-check the generated hook scripts:

```sh
.cache/tmact hook init zsh | zsh -n
.cache/tmact hook init bash | bash -n
.cache/tmact hook init fish | fish -n
```

Last run 2026-07-07: all of the above passed (fish skipped, not installed).

Last run 2026-07-08: `hook state` / `hook doctor` round-trip verified against an
isolated statusd (short `/tmp` socket, `web_addr:""`) — active→completed state
reflected, doctor healthy/unhealthy exit codes correct, no panes or rc files
touched.

## statusd web UI (manual / browser)

The React UI's layout-dependent behavior is not unit-testable (jsdom has no
layout engine — `scrollHeight`/`clientHeight` are 0), so verify these in a real
browser. Build the UI first (`make web`), then run statusd with a web address:

```sh
.cache/tmact statusd start --web-addr 127.0.0.1:7890
```

Short-pane no-scroll + bottom bars pinned (most telling at a narrow ~390 px
viewport; install as a PWA to exercise real safe-area insets):

1. Select a pane idling at a shell prompt (a few real lines; tmux pads the rest
   of the grid with blank rows).
2. Expect: `#content` does NOT scroll (`scrollHeight <= clientHeight`), output is
   top-aligned, and `nav.statusline` + `.input-bar` stay visible (they do not
   overflow the `overflow:hidden` body).

Long-pane stick-to-bottom:

3. Select a pane with 400+ lines of output.
4. Expect: `#content` scrolls and stays pinned to the bottom
   (`scrollTop + clientHeight ≈ scrollHeight`), newest line visible.

Boot placeholder (fresh load, no saved selection): on a narrow viewport the
`#draft` placeholder reads "Type a prompt, then tap Send" (not the desktop
⌘/Ctrl hint) and the mode strip shows "Select a pane to enable input".

Markdown table view (bottom-left `#markdown-btn` toggle):

5. Select a pane whose output is a pipe-delimited table (aligned `a | b | c`
   rows; a GitHub `---|---` row is optional — without it every row is a body row).
6. Tap the toggle. Expect: the pipe block becomes a bordered `table.tui-table`;
   non-table lines (totals, the shell prompt) stay as raw terminal text below it.
7. Tap again → back to raw pipes. `.active` highlight and
   `localStorage["tmact.settings"].markdownView` track the state across reloads.
   Default is off, so the first paint is always the raw terminal view.

Live pane interaction stability:

8. Select an isolated pane that continuously prints a changing counter and an
   existing image path. While it is updating, select the path text and copy it.
   Expect the selection and its DOM nodes to remain intact, with the accessible
   “Live updates paused while selecting” indicator visible.
9. Collapse the selection. Expect the indicator to disappear and only the
   newest pending frame to render once. Then hold pointer-down on a previewable
   path while output changes and Ctrl/Cmd-click it. Expect the click target to
   remain connected through click dispatch and the preview to open.

Last run 2026-07-22: verified steps 8–9 with `borz` against a rapidly changing
isolated local tmux pane. Range text copied intact; the path node kept identity
while frames arrived; selection collapse flushed the latest frame; and the
preview opened before the deferred repaint replaced the clicked node.

### Train infinite journey

Select the train pane-switcher layout before opening a journey URL. The
development-only controls are:

- `train-route-seed=<text>` selects the deterministic, versioned route seed
  (trimmed to 64 characters).
- `train-world-debug=1` shows direction, route position, visible near-chunk
  indices, near mounted count, total mounted count across all five parallax
  layers, and station state.
- `train-cruise-speed=<px-per-second>` overrides cruise speed, capped at 96.
- `train-station-trigger=approach|depart` starts near the next deterministic
  station event.

Use the existing Vite server and a reproducible URL such as:

```text
http://127.0.0.1:5234/?train-world-debug=1&train-route-seed=smoke-line&train-cruise-speed=96&train-station-trigger=approach
```

#### Required viewport protocol

Run the following sequence from the beginning for **every** requested size.
Do not reuse a screenshot or DOM result from a prior viewport, and do not set
the viewport as part of `borz open`:

1. Confirm the shared server returns HTTP 200 at the plain page URL.
2. Fetch the plain, non-cache-busted
   `http://127.0.0.1:5234/src/main.tsx` URL (no query string) and confirm the
   response contains the current entrypoint imports and source-map content.
   A stale or mismatched module invalidates the run.
3. Open or reopen the port-5234 journey URL with `borz` and wait for
   `.train-layout-world`.
4. Set the viewport with `borz viewport <width>x<height>` **after** opening.
5. Hard-reload with `borz refresh` and wait for `.train-layout-world` again.
6. Before inspecting DOM, waiting for a station state, or taking a screenshot,
   run `borz eval '({width: window.innerWidth, height:
   window.innerHeight})' --unwrap` and require an exact match. Stop and discard
   the evidence if either dimension differs.

Only after the dimension assertion passes should lazy train images be allowed
to settle and the visual/DOM checks begin. Repeat the complete sequence for:

- compact: 390×844
- desktop: 1280×800
- ultrawide: 2560×900

Final manual cases:

1. Using the required viewport protocol at 390×844, 1280×800, and 2560×900,
   verify the locomotive/carriages stay fixed while scenery travels right.
   Horizontal train inspection must not move the world.
2. Watch at least three region changes and a complete non-station set piece.
   Expect no blank seam, jump, obvious short repeat, or scenery that blocks
   train controls/passengers.
3. Let the station traverse `approach → decelerate → platform → dwell → depart
   → cruise`. Position must remain fixed during dwell while station steam and
   lights remain available.
4. Cycle day → sunset → night. Geometry and route position must remain stable;
   controls, passengers, terrain, water, and emissive overlays remain readable.
5. Resize compact → ultrawide → compact during cruise. The debug total may
   resize but must settle to a bounded value; repeated travel must not make it
   trend upward or leave a blank edge.
6. Switch train → office/bottom. The train DOM disappears and route position
   stops updating. Switch back to train with the same URL: seed, initial chunk
   geometry, and station trigger are reproduced.
7. During a sustained five-minute cruise, sample
   `.train-parallax-chunk`, `.train-scenery-asset`, animation-frame cadence,
   and browser task-manager CPU. Counts must remain bounded, controls remain
   responsive, and the console must stay free of errors.

Last run 2026-07-25: the shared Vite server returned HTTP 200 and the plain
`/src/main.tsx` response matched the current entrypoint before each viewport.
Each page was then opened/reopened, resized, hard-reloaded, and its actual
`window.innerWidth`/`window.innerHeight` asserted before evidence was recorded.
No private pane names, output, or payloads were included.

| Evidence size | Actual viewport | Station evidence | Bounded DOM evidence |
| --- | --- | --- | --- |
| Compact | 390×844 | `approach → decelerate → platform → dwell`; platform and dwell stayed at `3680px`; two station segments were visible behind the train | 31 parallax chunks at dwell and after departure/cruise; scenery assets recycled from 21 to 24 |
| Desktop | 1280×800 | Same complete transition and `3680px` stop; four station segments were visible behind the train | 44 parallax chunks and 29 scenery assets at dwell |
| Ultrawide | 2560×900 | Same complete transition and `3680px` stop; four station segments were visible with no seam or control collision | 64 parallax chunks and 45 scenery assets at dwell |

The ultrawide worst-case bounded-DOM run then continued for 305 seconds at
96 px/s and sampled every five seconds (62 samples). Route position advanced
from `4083.389px` to `28158.397px` across repeated station states. Both
`.train-parallax-chunk` and the debug total stayed exactly 64 throughout;
`.train-scenery-asset` recycled within 38–47 rather than trending upward.
Approach/dwell and sustained-run screenshots showed no blank edge or station
occlusion of the fixed controls, and `borz errors --json` reported no page
errors.

TRAIN-016 track run 2026-07-25: repeated the complete freshness and viewport
protocol at 390×844, 1280×800, and 2560×900. Each size mounted exactly one
`.train-world-track` directly under `.train-layout-world`, outside
`.train-layout-inspection`. Its bounded overscan covered both viewport edges
at every size, and scrolling train inspection left the track position and DOM
identity unchanged. During desktop cruise, route and track both advanced
`28.8px` over a 300ms sample at 96 px/s. Compact and ultrawide dwell samples
held both at `3680px`; departure resumed without a track edge or discontinuity.
The ultrawide scene retained 64 bounded parallax chunks, screenshots showed
continuous wheel alignment and sleepers from compact through ultrawide, and
`borz errors --json` reported no page errors.

TRAIN-018 height run 2026-07-25: repeated the complete freshness and viewport
protocol at 390×844, 1280×800, and 2560×900, switching between train and office
after each exact viewport assertion. The train and office outer heights matched
at `149.25px` compact and `184.078125px` desktop/ultrawide. The bottom-anchored
train artwork retained its `132px`, `144px`, and `160px` height bands while the
world gained `17.25px`, `40.078125px`, and `24.078125px` of visible sky,
respectively. The track/wheel baseline and initial horizontal inspection
position stayed fixed; screenshots showed continuous scenery with no blank
band, clipped consist, or altered office composition. `borz errors --json`
reported no page errors, and no pane output or private identifiers were
recorded in this evidence.

## Notes Template

```text
Date:
Command:
Target:
Result:
Follow-up:
```
