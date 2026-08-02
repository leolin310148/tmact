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
.cache/tmact workflow example --profile agent-dev > tmact-agent-dev-workflow.yaml
.cache/tmact workflow validate --config tmact-agent-dev-workflow.yaml --var request=demo
.cache/tmact workflow plan --config tmact-agent-dev-workflow.yaml --var request=demo
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

TRAIN-019 scale and hit-target run 2026-07-25: repeated the complete freshness
and viewport protocol at 390×844, 1280×800, and 2560×900 with mixed occupied
seats and empty filler carriages. The carriage and locomotive artwork measured
exactly `0.9` of their post-TRAIN-018 containers, while every carriage bottom
remained equal to the unchanged world-track top. Passenger targets measured
44×44px at compact and desktop sizes and 49.547×49.547px at ultrawide; no target
pairs overlapped, and `elementFromPoint` at all four inset corners of every
occupied target resolved to that intended passenger. The seat focus ring
remained a separate 2px outline, and the locomotive trigger stayed larger than
44px. Filler recalculation produced 2, 6, and 10 total carriages respectively;
the last ultrawide carriage extended beyond the 2560px inspection edge without
moving or exposing the world-owned track. Screenshots showed readable
empty/occupied windows and consistent bottom anchoring, and `borz errors
--json` reported no page errors.

TRAIN-021 shallow-perspective track run 2026-07-25: repeated the complete HTTP
200, plain-module freshness, open, viewport, hard-reload, and actual-dimension
protocol at 390×844, 1280×800, and 2560×900. The two low rail bands and diagonal
sleepers remained aligned below every wheel without restoring a tall ballast
wall. Each size kept exactly one world-owned track node outside train
inspection, with 31, 44, and 64 bounded parallax chunks respectively. Scrolling
desktop train inspection preserved the track node and its independent route
position. During a five-second ultrawide traversal, route and track advanced by
the same `410.699px`; all sampled track bounds covered both viewport edges, the
64-chunk count stayed constant, and no blank seam or visible aliasing jump
appeared. Deterministic station departure resumed into cruise with route and
track positions aligned, and `borz errors --json` reported no page errors.

TRAIN-022 route-driven wheel run 2026-07-25: repeated the complete HTTP 200,
plain-module freshness, open, viewport, hard-reload, and actual-dimension
protocol at 390×844 and 1920×900. Compact mounted 2 filler carriages and 11
wheel rims; wide mounted 8 filler carriages and 35 rims. In both cases the
declared bounded count exactly matched the DOM count, every rim remained
pointer-inert, and measured rim centers matched their sprite-relative centers
(the wide worst-case error was under 0.015px). At station dwell, route position
held at `3680px` and the shared wheel angle held at `-27.608deg` across a 250ms
sample. Departure and 96px/s cruise resumed both values together; a 300ms wide
cruise sample advanced from `4332.858px` to `4361.658px`. Compact and wide
screenshots showed crisp code-native rims integrated with the locomotive and
bogies, aligned over the moving perspective track without rotating bodies,
passengers, hit targets, or focus geometry. No private pane identifiers or
output were recorded in this evidence, and `borz errors --json` reported no
page errors.

TRAIN-023 natural-cloud run 2026-07-25: repeated the complete HTTP 200,
plain-module freshness, open, viewport, hard-reload, and actual-dimension
protocol for multiple seeds at 390×844 and 2560×900. `natural-clouds-a`
displayed a loose group using all three cloud variants, while
`harbor-weather` placed the compact viewport inside an intentional open-sky
gap. At ultrawide, `highland-front` mounted 8 clouds across 12 bounded sky
chunks, with 7 visible clouds, 3 group members, altitudes from 11.467% to
35.640%, and horizontal gaps from 189.781px to 1055.438px. The complete scene
stayed at 64 parallax chunks and 38 scenery sprites; cloud nodes never exceeded
the two-per-chunk bound. Screenshots masked all content above the train scene
and showed varied heights, scales, spacing, and open areas without a repeated
row, chunk-edge rhythm, seam, or control collision. `borz errors --json`
reported no page errors.

TRAIN-026 consist/track-overlap run 2026-07-25: repeated the complete HTTP 200,
plain-module freshness, open, viewport, hard-reload, and actual-dimension
protocol at 390×844 and 1920×900. Both sizes resolved the single shared consist
offset to `4px` against the unchanged `19px` world-owned track plane. Every
visible locomotive/carriage bottom stayed on the same baseline, while visible
wheel rims extended between roughly 3.7px and 8.2px into the track's upper
portion as their route-driven rotation changed. All rims remained unclipped,
the track covered the viewport, and horizontal train inspection preserved the
track node, route position, and geometry. Compact seat targets stayed at least
44×44px and wide targets at least 49.547×49.547px. Day and night screenshots
showed legible wheels seated on the rails with unchanged couplers, train scale,
sky area, and controls; `borz errors --json` reported no page errors. No private
pane names, output, or payloads were recorded.

TRAIN-038 scenery-balance run 2026-07-26: repeated the complete HTTP 200,
plain `/src/main.tsx` freshness, open, viewport, hard-reload, and exact
`window.innerWidth`/`window.innerHeight` protocol at 390×844, 1280×800, and
2560×900. Deterministic `train-038-town-edge` positions displayed both
town-edge variants without a near-layer station overlap. For every viewport,
the fixed train and debug overlay were hidden only in the live DOM, day and
night screenshots were captured, and the DOM was restored before the next
check. All visible town-edge segments had computed opacity `1`, their solid
background colors contained no fractional alpha, and the screenshots showed
opaque low-contrast building faces, hard-edged roof-height variation,
architectural window/ledge detail, and intentional sky gaps instead of
translucent near-track rectangles. The twelve retained screenshots are
`/private/tmp/train-038-town-edge-v{0,1}-{compact,desktop,ultrawide}-{day,night}.png`.

The ultrawide `train-038-sustained` traversal then ran at 96 px/s for 418,659 ms
between its first and last of 60 five-second samples. Route position advanced
from `3967.152px` to `37815.585px`; `.train-parallax-chunk` stayed exactly 64
while `.train-scenery-asset` recycled within 28–39. Night, day, and sunset were
all sampled. The natural run covered town, industrial, coast, and mountain;
deterministic position checks added forest and the bridge, tunnel, and
unobscured coast-reveal compositions in day and night, complementing the
town-edge and station coverage. A 100 ms station sample captured `approach →
decelerate → platform → dwell → depart → cruise`; platform and dwell both held
at `3680px`. A normal refresh without a position or station override restored a
nonzero same-seed cruise snapshot with `data-journey-restored="true"` and
advanced only `0.001px` over the next second at the requested 0.001 px/s.
Manual palette state was not restored. `borz errors --json` reported no page
errors, and no private pane names, output, or payloads were recorded.

TRAIN-039 town-edge and sky-motion run 2026-07-26: repeated the HTTP 200,
plain `/src/components/TrainLayout.tsx` freshness, open, viewport, refresh, and
exact `window.innerWidth`/`window.innerHeight` protocol at 390×844, 1280×800,
and 2560×900 with the dedicated `tmact-train-workitems` borz profile. The fixed
`.train-layout-inspection` and diagnostics overlay were hidden only in the live
DOM and restored after every check. Both deterministic town-edge variants
rendered the continuous entry/body/exit sequence from nine opaque rowhouse,
apartment, cottage, and church sprites with material and roof variation,
aligned night-window masks, intentional gaps, and computed set-piece
`background-image: none`. Day and night checks had no visible near-station
overlap or anonymous rectangular settlement band. The twelve screenshots are
`/private/tmp/train-039-town-edge-v{0,1}-{compact,desktop,ultrawide}-{day,night}.png`.

At 96 px/s, a 10.521-second desktop sample measured route/near displacement
of 1009.536px, sun 4.038px, wisp 20.190px, routed cloud 60.572px, and far
terrain 252.384px, proving `0 < sun < wisp < cloud < far < near`; the layer
ratios resolved to 0.004, 0.02, 0.06, 0.25, and 1 while the route window stayed
bounded at 45 chunks. The DEV-only reduced-motion diagnostic held a restored
4096px route plus every sun, wisp, and cloud position unchanged for 2.5
seconds, with all anchor motion distances at zero. A triggered station run
entered dwell after 9.831 seconds at 3680px and held route, sun, wisp, cloud,
far, and near positions unchanged for the next second. `borz errors --json`
reported no page errors; the live DOM was restored and no private pane names,
output, or payloads were recorded.

TRAIN-040 shaped-terrain run 2026-07-26: repeated the HTTP 200, plain
`/src/components/TrainLayout.tsx` source/source-map freshness, open, viewport,
hard-reload, and exact `window.innerWidth`/`window.innerHeight` protocol at
390×844, 1280×800, and 2560×900 with the dedicated
`tmact-train-workitems` borz profile. Deterministic positions for town-edge,
coast/station, mountain/bridge, industrial, coast-reveal, tunnel, and forest
covered all five regions and every requested set piece. A temporary live-DOM
stylesheet hid `.train-layout-inspection`, `.train-world-debug-grid`, and
`.train-time-toggle` for each capture and was explicitly removed afterward;
the restored displays were `flex`, `block`, and `grid`.

Day and night computed-style checks found zero visible non-sky chunks retaining
a background image or solid background color. Every visible large-area owner
was instead an opaque, textured `.train-terrain-base` with a polygon contour
and an explicit region material. The compact, desktop, and ultrawide contact
sheets showed continuous shaped upper edges without a broad uniform block,
vertical chunk walls, sky leaks, or floating scenery. The 42 retained source
screenshots are
`/private/tmp/train-040-{town-edge,coast-station,mountain-bridge,industrial,coast-reveal,tunnel,forest}-{compact,desktop,ultrawide}-{day,night}.png`;
the inspection-only contact sheets are
`/private/tmp/train-040-{compact,desktop,ultrawide}-contact.png`.
The final ultrawide sample retained 64 bounded parallax chunks, 52 non-sky
chunks, exactly 52 terrain owners, and 38 scenery sprites. `borz errors --json`
reported no page errors, and no private pane names, output, or payloads were
recorded.

TRAIN-041 whole-station run 2026-07-26: repeated the HTTP 200, plain
`/src/main.tsx` import/source-map freshness, current
`/src/components/TrainLayout.tsx` station-wrapper source, open, viewport,
hard-reload, and exact `window.innerWidth`/`window.innerHeight` protocol at
390×844, 1280×800, and 2560×900 with the dedicated
`tmact-train-workitems` borz profile. The `infinite-journey` first station was
held at `3680px` and `4480px` with a `0.001px/s` diagnostic speed so its entry
and exit remained stable for day, sunset, and night captures.

A temporary live-DOM stylesheet hid `.train-layout-inspection`,
`.train-world-debug-grid`, and `.train-time-toggle` for every capture and was
removed afterward; their restored displays were `flex`, `block`, and `grid`.
Computed-style inspection across all six `entry/body/body/body/body/exit`
segments found `clip-path: none` and `overflow: visible` on every station and
architecture wrapper. Only the 20px-high entry and exit platform-transition
wrappers owned polygon clips. Building, canopy, platform, lamp, and signal
bounding boxes remained rectangular and aligned at both diagnostic positions.

The 18 retained screenshots are
`/private/tmp/train-041-{entry,exit}-{compact,desktop,ultrawide}-{day,sunset,night}.png`.
They show clean station joins and whole façades, roofs, canopies, name board,
lamps, illuminated windows, and platforms without diagonal cuts, triangular
sky holes, duplicate walls, floating columns, or track gaps. `borz errors
--json` reported no page errors, and no private pane names, output, or payloads
were recorded.

TRAIN-042 station-compositor audit 2026-07-26: repeated HTTP 200, plain
`/src/main.tsx` and `/src/components/TrainLayout.tsx` module freshness, open,
viewport, hard-reload, and exact `window.innerWidth`/`window.innerHeight`
checks at 390×844, 1280×800, and 2560×900 with the dedicated
`tmact-train-workitems` borz profile. The `infinite-journey` station was held
near route positions `3680px` and `4480px` at `0.001px/s`; the final
post-build freshness pass again loaded the current station composition source
and found all six expected segments at 2560×900.

Computed-style inspection in day, sunset, and night found every station wall
at `opacity: 1` with an opaque resolved `color(srgb …)` background and
`filter: none`. Sorted wall bounding boxes overlapped adjacent segments by
8px, replacing the former 41.34px reveal widths without changing the
entry/exit platform ramps. The composition owns six distinct bays
(`entrance`, `west-waiting`, `ticket-hall`, framed `platform-view`,
`freight-office`, and `departure`), seven role-distributed canopy supports,
four platform lamps, and only the segment-0 approach plus segment-5 departure
signals. The framed platform view remained station-owned and opaque instead
of exposing a random far-layer sprite.

All nine station emissive overlays computed to opacity `0` and `filter: none`
in day. Sunset lit only the four `sunset-night` fixtures at opacity `0.24`;
night lit the deliberately sparse three window banks, four lamps, and two
operational signal aspects, with illumination still owned by the corresponding
physical fixture. The ultrawide route window remained bounded at 65 chunks.
Additional deterministic samples confirmed opaque non-cloud sprites for
forest, mountain, town, coast, and industrial scenery, including industrial
midground buildings and far/ultra-far terrain.

For every route position, palette, and viewport, the complete station plus
isolated sky, ultra-far, far, midground, and near evidence is retained as 108
screenshots:
`/private/tmp/train-042-{entry,exit}-{compact,desktop,ultrawide}-{day,sunset,night}{,-sky,-ultra-far,-far,-midground,-near}.png`.
The contact inspection showed coherent wall/roof/canopy joins, intentional
architectural ends, unlit daylight fixture glass, restrained sunset light,
sparse night light, and no mountains, trees, or unrelated buildings embedded
inside station walls. The temporary live-DOM styles were removed; restored
displays were `flex`, `block`, and `grid` for the inspection, debug grid, and
time toggle. `borz errors --json` reported no page errors, and no private pane
names, output, or payloads were recorded.

TRAIN-043 set-piece choreography run 2026-07-26: used route seed
`train-043-proof` and the dedicated `tmact-train-workitems` borz profile at
375×812, 1280×800, and 2560×1080. Every capture repeated HTTP 200, confirmed
the plain non-cache-busted `/src/components/trainRoute.ts` contained the
current projection source, opened the port-5234 page, applied the viewport,
requested a hard reload, waited for the reloaded train controls, and asserted
the exact `window.innerWidth` and `window.innerHeight`.

The deterministic `train-set-piece-focus` diagnostic centred the first
`bridge`, `tunnel`, `town-edge`, `coast-reveal`, and `station` occurrence.
For all 15 captures, the actual render-layer geometry—not route metadata—had
the complete expected segment-ID set and a meaningful non-zero bounding box.
Every union centre was exactly 50% of the viewport width. Visible union widths
were 375px at compact size, 964–1280px on desktop, and 964–1924px ultrawide,
all above `min(320px, 50% of viewport width)`. The synchronized projection
overlay remained bounded at 34 segments or fewer while ordinary route chunks
retained their existing bounded windows. Projected reservations suppressed
ordinary scenery in intersecting layer-space chunks.

The train plus `.train-layout-inspection`, `.train-world-debug-grid`, and
`.train-time-toggle` were temporarily hidden before each day screenshot and
restored afterward. The retained evidence is:
`/tmp/tmact-train-043-proof/{compact,desktop,ultrawide}-{bridge,tunnel,town-edge,coast-reveal,station}-day.png`;
the inspected contact sheets are
`/tmp/tmact-train-043-proof/contact-{compact,desktop,ultrawide}-day.png`.
Visual inspection confirmed that each named composition was centred and
recognizable rather than an edge sliver or metadata-only pass. `borz errors`
was empty; the console contained only Vite connection and React development
messages. Focused route/scenery/layout tests, all 432 frontend tests,
production build, and `make test` passed.

TRAIN-044 station-campus run 2026-07-26: used route seed
`train-044-proof` and the dedicated `tmact-train-workitems` borz profile at
390×844, 1280×800, and 2560×900. Every viewport repeated HTTP 200, fetched the
plain non-cache-busted `/src/components/TrainLayout.tsx` module and found the
current campus source, opened the port-5234 page, applied the viewport, issued
a hard reload, and asserted the exact `window.innerWidth` and
`window.innerHeight`.

The first station was inspected at entry, centre, and exit. The centred route
positions were respectively `2915/3715/4515px` on compact,
`3360/4160/4960px` on desktop, and `4000/4800/5600px` on ultrawide. Each
capture hid the train through `.train-layout-inspection` together with
`.train-world-debug-grid` and `.train-time-toggle`; a second capture isolated
the near layer. The temporary styles were removed afterward and the three
elements restored to `flex`, `block`, and `grid`.

Visual inspection found one continuous six-segment platform campus with
visible entry and exit ramps, three deliberately small opaque buildings
(gatehouse, station house, and service shed), six differently sized canopy
bays, five framed openings that expose the actual parallax world, and
role-owned benches, wayfinding, timetable, planter, baggage cart, and parcel
stack. Compact entry/centre/exit images retained architectural ends. Desktop
and ultrawide images read as one varied campus rather than repeated tiles; the
isolated near layer confirmed that openings were genuine gaps and that no
façade or canopy formed a viewport-spanning solid wall. All building widths
were at most `207.38px`; canopy widths ranged from `230.06px` to `278.66px`.

Computed styles confirmed three opaque buildings, six opaque canopies, six
opaque platforms, nine supports, seven lamps, two signals, and eight service
elements, with the station remaining pointer-inert. Stable palette captures
and a separate normal-transition terminal-state check found all 11 emissive
overlays at opacity `0` with no filter in day, only five station-owned sunset
fixtures at opacity `0.24`, and all owned fixtures at opacity `0.48–0.66` at
night. The 54 retained screenshots are
`/tmp/tmact-train-044-proof/{compact,desktop,ultrawide}-{entry,center,exit}-{day,sunset,night}-{full,near}.png`;
the six inspected contact sheets are
`/tmp/tmact-train-044-proof/contact-{compact,desktop,ultrawide}-{full,near}.png`.
`borz errors --json` was empty and the console contained only Vite connection
and React development messages.

TRAIN-045 terrain-silhouette run 2026-07-26: used route seed
`train-045-proof` and ordinary, set-piece-free central chunks for town `4`,
mountain `57`, coast `94`, industrial `129`, and forest `257`. The dedicated
`tmact-train-workitems` borz profile was validated at 390×844 and 1280×800.
For each viewport, the run confirmed HTTP 200, fetched the plain
non-cache-busted `/src/components/TrainLayout.tsx` module and found the current
terrain envelope/material source, opened the port-5234 page, applied the
viewport, reloaded, and asserted the exact `window.innerWidth` and
`window.innerHeight`.

Day acceptance isolated ultra-far, far, midground, and near for all five
regions at both viewport sizes. Each parallax position was calculated from
the owning layer's `0.1`, `0.25`, `0.55`, or `1` speed ratio; the target chunk
was visibly centred at x `195px` compact or `640px` desktop with a `322px`
bounding width. The retained 40 isolated screenshots are
`/private/tmp/train-045-proof/{compact,desktop}-{forest,mountain,town,coast,industrial}-day-{ultra-far,far,midground,near}.png`.
Ten compact and ten desktop full composites cover every region in sunset and
night at
`/private/tmp/train-045-proof/{compact,desktop}-{forest,mountain,town,coast,industrial}-{sunset,night}-composite.png`.

The four inspected contact sheets are
`/private/tmp/train-045-proof/contact-{compact,desktop}-day-layers.png` and
`/private/tmp/train-045-proof/contact-{compact,desktop}-composites.png`.
Inspection found continuous irregular silhouettes, exact joins without sky
holes or chunk seams, a stable filled near track bed, and distinct forest
soil, mountain rock, town ground, coast shore, and industrial fill patterns.
Mountain relief was deliberately tallest, coast formed a low varied shore
profile, and the four depth planes overlapped as terrain rather than reading
as stacked near-horizontal colour strips. Existing scenery sprites were left
for their dedicated later work items.

Every capture hid `.train-layout-inspection`, `.train-world-debug-grid`, and
`.train-time-toggle`, then restored them. The final displays were `flex`,
`block`, and `grid`, with zero temporary hidden markers. `borz errors --json`
was empty; the console contained only Vite connection and React development
messages. The focused layout suite passed 78 tests, the complete frontend
suite passed all 432 tests, the production build passed, and `make test`
passed.

TRAIN-046 depth-and-grounding run 2026-07-26: used route seed
`train-046-proof` and ordinary central chunks town `23`, coast `70`, mountain
`106`, forest `160`, and industrial `268`. Each chunk and both immediate
neighbours were in the same region and free of set pieces; the centred chunk
also had the deterministic `dense` composition role. The dedicated
`tmact-train-workitems` borz profile used one explicit tab at 390×844,
1280×800, and 2560×900. Every viewport repeated HTTP 200, fetched the plain
non-cache-busted `/src/components/TrainLayout.tsx` module and found the current
depth/grounding/illumination ownership source, opened the port-5234 page, set
the viewport, reloaded, and asserted exact `window.innerWidth` and
`window.innerHeight`.

All 15 day and 15 night composites hid the fixed train through
`.train-layout-inspection` together with `.train-world-debug-grid` and
`.train-time-toggle`. The central near-layer chunk was the requested region,
reported `setPiece:none`, and had a zero measured anchor error in every
viewport. Ultrawide stayed bounded at 65 ordinary route chunks. Attached
emissive overlays had a matching unique scenery instance and no orphan owner.
The retained composites are
`/private/tmp/train-046-proof/{compact,desktop,ultrawide}-{town,coast,mountain,forest,industrial}-{day,night}-composite.png`.

Twelve isolated day captures centred the same ordinary town chunk independently
in ultra-far, far, midground, and near coordinates at x `195px`, `640px`, and
approximately `1280px`. The measured layer grammar was respectively scale
multiplier/contrast/detail budget `0.58/0.68/1`, `0.78/0.80/2`,
`0.96/0.94/3`, and `1.20/1.10/4`. The retained layer evidence is
`/private/tmp/train-046-proof/{compact,desktop,ultrawide}-town-day-{ultra-far,far,midground,near}.png`.
The six inspected contact sheets are
`/private/tmp/train-046-proof/contact-{compact,desktop,ultrawide}-{composites,layers}.png`.

Contact-sheet and representative-original inspection found progressively
larger, darker, higher-contrast silhouettes toward the train; small pale
distant ridges no longer shared the same scale as far terrain. Buildings,
vegetation, poles, and props met their owning contour at the visible opaque
base instead of floating, sinking, or sitting on the rail surface. Near props
remained sparse and did not obscure the hidden train band. Night masks and
regional light details remained attached to their physical owner geometry.
Atmospheric haze remained on the two dedicated compositor veil planes; no
sprite owned haze. No blurred edges, matte fringes, new alpha fill, gaps, or
seams were observed.

The temporary inspection stylesheet was removed afterward. The train
inspection and time toggle restored to `flex` and `grid`; all five world layers
and both depth veils restored to `block`, while the disabled debug grid remained
absent. `borz errors --json` was empty. The focused scenery/layout/terrain
suite passed 104 tests, the complete frontend suite and production build
passed, and `make test` passed.

TRAIN-047 forest-and-mountain composition run 2026-07-26: used the dedicated
`tmact-train-workitems` borz profile at 390×844, 1280×800, and 2560×900. The
ordinary forest samples used seed/position `train-047-proof/22754.545`,
`ordinary-pools-a/404654.545`, and
`regional-rhythm-cedar/123054.545`; ordinary mountain used
`train-047-proof/299118.182`, `ordinary-pools-a/426181.818`, and
`train-047-proof/65454.545`. Forest-to-mountain transitions used
`train-047-proof` at `99263.636`, `100072.727`, and `101236.364`; the reverse
transitions used `235409.091`, `236218.182`, and `237381.818`.

Every requested viewport confirmed HTTP 200, fetched the plain non-cache-busted
`/src/components/TrainLayout.css` module and found the current forest/mountain
palette and role source, opened the port-5234 page, applied the viewport, issued
a hard reload, and asserted the exact `window.innerWidth` and
`window.innerHeight`. The final source recheck also resolved the forest-soil
and mountain-rock palette variables to `#315944` and `#706b7b`.

The 24 full day/night captures and 24 matching midground-isolated captures are
`/private/tmp/train-047-proof/{compact,desktop,ultrawide}-{forest,forest-to-mountain,mountain,mountain-to-forest}-{day,night}{,-midground}.png`.
The 12 inspected contact sheets are
`/private/tmp/train-047-proof/contact-{compact,desktop,ultrawide}-{day,night}{,-midground}.png`;
their order is forest, forest-to-mountain, mountain, then mountain-to-forest.
Inspection found clustered deciduous/conifer forest silhouettes separated by
clearings, streams, undergrowth, and fence beats; mountain views alternated
layered ridges, cliffs, rock fields, alpine scrub, and open vistas. Both
directions of transition changed from green forest soil to gray-purple rock
without mirrored wallpaper, floating anchors, flat slabs, or repeated
viewport-wide bands.

Measured forest unions were 218.4–257.5px wide, mountain compositions were
962–1922px wide and 153.4–183.1px tall, and transition compositions were
642–3202px wide and 148.3–183.1px tall. The ultrawide ordinary route window
remained bounded at 64 chunks. The temporary train, debug, control, layer,
veil, and projection hiding was fully removed: the inspection and time toggle
restored to `flex` and `grid`, all five world layers and both veils restored to
`block`, all 18 projected segments were visible, and no temporary markers
remained. `borz errors --json` was empty; the console contained only Vite and
React development messages.

The focused scenery/layout suites passed all 105 tests, the complete frontend
suite passed all 439 tests, the production build passed, and `make test`
passed.

TRAIN-048 town-and-industrial composition run 2026-07-27: used route seed
`train-048-proof` with town at `1809.091`, `2618.182`, and `3781.818`;
town-to-industrial at `5009.091`, `5818.182`, and `6981.818`; industrial at
`8790.909`, `9600`, and `10763.636`; and industrial-to-town at `10245.455`,
`11054.545`, and `12218.182` for compact, desktop, and ultrawide respectively.
The dedicated `tmact-train-workitems` borz profile used exact 390×844,
1280×800, and 2560×900 viewports.

Every viewport confirmed HTTP 200, fetched the plain non-cache-busted
`/src/components/TrainLayout.tsx` module and found the current
`townhouse-block` and `industrial-shed` fixture source, opened the port-5234
page, set the viewport, issued a hard reload, and asserted exact
`window.innerWidth` and `window.innerHeight`. The train, debug controls, and
projected set-piece segments were temporarily hidden so each ordinary sequence
showed the regional grammar unobstructed.

The 36 retained full captures are
`/private/tmp/train-048-proof/{compact,desktop,ultrawide}-{town,town-to-industrial,industrial,industrial-to-town}-{day,sunset,night}.png`.
The nine inspected contact sheets are
`/private/tmp/train-048-proof/contact-{compact,desktop,ultrawide}-{day,sunset,night}.png`;
their order is town, town-to-industrial, industrial, then
industrial-to-town. Town scenes formed opaque settlement blocks from
townhouses, storefront awnings, streets, yards, fences, trees, civic clocks,
and yard gates. Industrial scenes used sheds, tanks, furnace and vent stacks,
cranes, utility poles and lines, service pipes, service roads, and deliberate
open-yard beats. Both transition directions changed composition gradually
without mirrored wallpaper, isolated track-line props, floating anchors, or
repeated identical façades.

The ultrawide midground route window remained bounded at 13 chunks. Visible
town fixtures covered fence, street-tree, townhouse-block, yard-gate,
civic-clock, and shop-awning; industrial fixtures covered industrial-shed,
utility-pole, service-pipe, gantry-crane, furnace-stack, vent-stack, and
storage-tank. Solid surfaces stayed opaque. Attached windows, lamps, signals,
and furnace lights were absent by day, localized at sunset, and strongest at
night while retaining matching owner geometry.

The temporary stylesheet was removed afterward. The train inspection and time
toggle restored to `flex` and `grid`, all 24 projected segments restored
visible, and no temporary marker remained. `borz errors --json` was empty.
The focused scenery/layout/asset suites passed all 118 tests, the complete
frontend suite passed all 444 tests, the production build passed, and
`make test` passed.

TRAIN-049 coast composition run 2026-07-27: used route seed
`train-049-proof`, coast region `28`, and ordinary coast chunks `256`, `258`,
and `260` for arrival, ordinary travel, and departure. Their centred journey
positions were respectively `149009.091`, `150172.727`, and `151336.364` at
390×844; `149818.182`, `150981.818`, and `152145.455` at 1280×800; and
`150981.818`, `152145.455`, and `153309.091` at 2560×900.

Every viewport confirmed HTTP 200, fetched the plain non-cache-busted
`/src/components/TrainLayout.css` module and found the current coast shore and
water-plane source, reopened the fixed dedicated tab, set the viewport, issued
a hard reload, and asserted exact `window.innerWidth` and
`window.innerHeight`. The train, debug controls, time toggle, and projected set
pieces were temporarily hidden. Each named chunk had a 322px visible bounding
box centred at x `195`, `640`, or `1280`; the visible water union was 128px
deep, had 4–11 participating planes plus matching visible shore profiles, and
extended beyond both viewport edges through the arrival, ordinary, and
departure sequence. Ultrawide remained bounded at 64 ordinary route chunks.

The 27 retained captures are
`/private/tmp/train-049-proof/{compact,desktop,ultrawide}-{arrival,ordinary,departure}-{day,sunset,night}.png`.
The nine inspected contact sheets are
`/private/tmp/train-049-proof/contact-{compact,desktop,ultrawide}-{day,sunset,night}.png`;
their order is arrival, ordinary travel, then departure. Inspection found a
broad continuous palette-owned sea, separate far and midground movement cues,
irregular opaque beaches and rock shelves, low coastal horizons, and visible
piers, boats, buoys, harbour posts, and dune grass. Coast no longer depended on
metadata, white horizontal bars, repeated water fragments, vertical colour
blocks, or mountain-dominated ordinary views. Day, sunset, and night retained
readable water depth and shoreline ownership; lighthouse reflection markup was
source-owned and clipped by its matching water mask.

The temporary stylesheet was removed afterward. The train inspection and time
toggle restored to `flex` and `grid`, all projected segments restored visible,
and no temporary marker remained. `borz errors --json` was empty. The focused
scenery/layout suites passed all 115 tests, the complete frontend suite passed
all 449 tests, the production TypeScript/Vite build passed, and `make test`
passed.

TRAIN-050 bridge-and-tunnel traversal run 2026-07-27: used route seed
`train-050-proof`. Bridge variant 0 used occurrence `0`, bridge variant 1 used
occurrence `2`, tunnel variant 0 used occurrence `1`, and tunnel variant 1 used
occurrence `0`. Compact centres were respectively `36035`, `87555`, `41635`,
and `7715`; desktop centres were `36480`, `88000`, `42080`, and `8160`;
ultrawide centres were `37120`, `88640`, `42720`, and `8800`. Bridge
entry/exit positions were the matching centre minus/plus `872.727`, while
tunnel entry/exit positions used minus/plus `581.818`.

Every requested scene confirmed HTTP 200, fetched the plain non-cache-busted
`/src/components/TrainLayout.tsx` module and found the current
`data-traversal-composition` source, opened an exact tab in the dedicated
`tmact-train-workitems` profile, set the viewport after opening, performed a
hard reload and a calibrated refresh, then asserted exact `390×844`,
`1280×800`, or `2560×900` `window.innerWidth`/`window.innerHeight` values.
The train inspection, debug grid, and time toggle were hidden only while
capturing each scene. The temporary stylesheet was removed and checked before
each exact proof tab was closed; all 29 earlier diagnostic proof tabs created
by this run were also closed by exact id.

The 72 retained captures are
`/private/tmp/train-050-proof/{compact,desktop,ultrawide}-{bridge,tunnel}-v{0,1}-{entry,centre,exit}-{day,night}.png`.
The six inspected contact sheets are
`/private/tmp/train-050-proof/contact-{compact,desktop,ultrawide}-{day,night}.png`;
within each sheet the rows are bridge v0, bridge v1, tunnel v0, and tunnel v1,
with centre, entry, and exit across the columns. Inspection found two distinct
continuous bridge grammars with visible approaches, track-contact decks,
lattice spans, crossing voids, lower supports, and exits. It also found two
distinct tunnel grammars with approach cuttings, round or stepped portals,
near-black openings, enclosing opaque mountain mass, passage lining, and
exits. A first visual pass exposed a day-palette white tunnel opening; the
opening was changed to a silhouette-owned near-black surface and all tunnel
captures were repeated before acceptance.

Bridge visual unions measured `1286×90px` and `1286×101px`; tunnel unions
measured `966×126px` and `966×138px`. Their centre captures landed exactly at
x `195`, `640`, and `1280`. Compact entry/exit captures retained about `358px`
of visible structure, desktop retained at least `803px`, and ultrawide showed
the complete union. Every traversal rendered matching primary midground and
supporting near-layer segment counts; centre-layer offsets were zero. The
ultrawide world remained bounded at 65 total mounted chunks.

`borz errors --json` was empty; console output contained only Vite connection,
React development, and expected hot-update messages. The focused route,
scenery, terrain-asset, and layout suites passed all 140 tests; the complete
frontend suite passed all 451 tests; the production TypeScript/Vite build and
`make test` passed.

TRAIN-051 town-edge and coast-reveal transition run 2026-07-27: used route
seed `train-051-proof` and the dedicated `tmact-train-workitems` borz profile.
Town-edge variant 1 used occurrence `1` (start chunk `18`) and variant 0 used
occurrence `5` (start chunk `90`). Coast-reveal variant 0 used occurrence `0`
(start chunk `180`) and variant 1 used occurrence `2` (start chunk `216`).
Every scene confirmed HTTP 200, fetched the plain non-cache-busted
`/src/components/TrainLayout.tsx` module, opened the port-5234 page in the
exact profile tab, set the viewport after opening, hard reloaded, and asserted
the exact `390×844`, `1280×800`, or `2560×900` inner dimensions.

Town entry/exit positions used the focus centre minus/plus `581.818px`, which
accounts for its `0.55` midground speed. Coast entry/exit used minus/plus
`1920px`, accounting for the four-segment role offset at its `0.25` far-layer
speed. Every entry, centre, and exit role landed exactly at x `195`, `640`, or
`1280`. Centre union visibility was `390/964/964px` for town and
`390/1280/1284px` for coast at compact/desktop/ultrawide; entry and exit
retained `324px` in every viewport, above the meaningful-object threshold.

Both town variants visibly progress from one open-edge building through three
gathering buildings to four settled-block buildings. Variant 0 uses a
market-road grammar and variant 1 a garden-lane grammar; both include opaque
roads, yards, gates, trees, foreground verges and fences, with their eight
sprite buildings grounded above the road instead of on the rail line. Both
coast variants use a broad opaque far-water plane, three depth cues,
midground shoreline framing and near foreground openings. Open-bay water
coverage is `58/100/100/62%`; harbour-mouth coverage is
`64/100/100/70%`. The obsolete per-segment `coast-shore` raster placement was
removed after the first inspected sample exposed repeated white bars and
cloned rocks.

The first final compact contact inspection exposed a station body/exit
colliding with coast entry despite logically disjoint route intervals.
Deterministic render-stage collision arbitration now removes incompatible
station/bridge/tunnel projections when a transition has meaningful visible
coverage during cruise, while an active station keeps priority. All 72 final
captures assert that no incompatible set-piece bounding box intersects the
target transition. Coast entry required one exclusion in both variants;
desktop/ultrawide coast v0 exit also required one. Route generation, region
order, focus anchors, station timing, and bounded projection windows remain
unchanged.

The 72 retained captures are
`/private/tmp/train-051-proof-20260727/{compact,desktop,ultrawide}-{town-edge,coast-reveal}-v{0,1}-{entry,centre,exit}-{day,night}.png`.
The six inspected contact sheets are
`/private/tmp/train-051-proof-20260727/contact-{compact,desktop,ultrawide}-{day,night}.png`.
Every capture temporarily hid `.train-layout-inspection`,
`.train-world-debug-grid`, and `.train-time-toggle`, then restored the live
DOM. Inspection found both transitions immediately legible in day and night,
with no station/traversal collision, metadata-only pass, legacy white shore
bar, floating building, or hidden water reveal. `borz errors --json` was
empty; console output contained only Vite connection and React development
messages. The focused route/scenery/terrain/layout suites passed all 144
tests, the complete frontend suite passed all 455 tests, the production
TypeScript/Vite build passed, and `make test` passed.

TRAIN-052 sky, atmosphere, and regional time-of-day run 2026-07-27: used
route seed `train-052-proof`, reduced-motion proof mode, and the dedicated
`tmact-train-workitems` borz profile. Ordinary region samples used chunk 33
for forest, 22 for mountain, 3 for town, 130 for coast, and 15 for
industrial. Compact route positions were respectively `10525`, `7005`, `925`,
`41565`, and `4765`; desktop positions were `10080`, `6560`, `480`, `41120`,
and `4320`; ultrawide positions were `11680`, `8160`, `2080`, `42720`, and
`5920`. Station, bridge, tunnel, town-edge, and coast-reveal used focus
occurrence `0`.

Each requested viewport independently confirmed HTTP 200, fetched the plain
non-cache-busted `/src/components/TrainLayout.tsx` module and found the
current horizon/palette source, reopened the exact proof tab, set the viewport
after navigation, performed a hard reload, and asserted `390×844`,
`1280×800`, or `2560×900` actual inner dimensions. Every capture temporarily
hid `.train-layout-inspection`, `.train-world-debug-grid`, and
`.train-time-toggle`; the proof style was removed afterward, with inspection
restored to `flex` and the time toggle to `grid`.

The 90 retained captures are
`/private/tmp/train-052-proof/{compact,desktop,ultrawide}-{forest,mountain,town,coast,industrial,station,bridge,tunnel,town-edge,coast-reveal}-{day,sunset,night}.png`.
The three inspected contact sheets are
`/private/tmp/train-052-proof/contact-{compact,desktop,ultrawide}.png`; each
row is one region or set-piece type and the columns are day, sunset, and
night. Day reads as clear blue with unlit fixtures, sunset keeps cooler depth
under a seed-owned low localized horizon glow instead of a blanket sepia
wash, and night preserves blue-black layer separation, sparse non-lattice
stars, varied celestial placement, and restrained owner-attached regional
lights. Forest, mountain, town, coast, and industrial night pools remain
visually distinct; coast reflection stays inside water ownership.

All five set-piece unions were centred inside the viewport's central half.
Their minimum visible union widths were `390px` compact and `964px` at both
desktop and ultrawide, above the `min(320px, 50% viewport width)` requirement.
Ordinary region samples also placed a meaningful region chunk in the central
half; the smallest measured region coverage was `801px` desktop and `964px`
ultrawide. Ultrawide stayed bounded at 64 mounted route chunks. Visual
inspection found readable solid geometry in every palette, with no blanket
recolour, daylight emissive pool, detached reflection, hidden named
composition, or alpha regression. `borz errors --json` was empty; console
output contained only Vite connection and React development messages. The
focused sky/star/scenery/layout suites passed all 131 tests, the complete
frontend suite passed all 458 tests, the production TypeScript/Vite build
passed, and `make test` passed.

TRAIN-053 final visual convergence run 2026-07-27: audited seeds
`train-053-aurora`, `train-053-cascade`, `train-053-harbour`,
`train-053-orchard`, and `train-053-summit` with the dedicated
`tmact-train-workitems` borz profile. Each compact (`390×844`), desktop
(`1280×800`), and ultrawide (`2560×900`) run independently confirmed HTTP
200, fetched the plain non-cache-busted `/src/components/TrainLayout.tsx`
module and matched the current collision-priority source, opened the proof
page, set the viewport, hard reloaded, and asserted actual `innerWidth` and
`innerHeight`. Every capture hid `.train-layout-inspection`,
`.train-world-debug-grid`, and `.train-time-toggle`; the proof style was
removed afterward, restoring inspection to `flex` and the time toggle to
`grid`.

The set-piece matrix used aurora bridge occurrences `0/2` (variants `1/0`),
cascade tunnel occurrences `0/1` (variants `1/0`), harbour town-edge
occurrences `0/1` (variants `0/1`), orchard coast-reveal occurrences `0/2`
(variants `1/0`), and summit station occurrence `0`. Compact focus positions
were respectively `7555/36035`, `162915/173475`, `355/6115`,
`127235/144515`, and `3715`; desktop positions were `8000/36480`,
`163360/173920`, `800/6560`, `127680/144960`, and `4160`; ultrawide
positions were `8640/37120`, `164000/174560`, `1440/7200`,
`128320/145600`, and `4800`. All nine compositions were captured in day,
sunset, and night at all three viewports. Compact visible unions were `390px`
centred at `195px`; desktop minimum union width was `964px` centred at
`640px`; ultrawide minimum union width was `964px` centred at `1280px`.

The audit caught one real integration regression before acceptance: at
ultrawide width, collision arbitration let a nearby transition remove the
explicitly focused aurora bridge and summit station while focus metadata
still claimed success. Explicit diagnostic focus now has arbitration
priority, so the bridge and station remain visible while the incompatible
coast-reveal or town-edge is excluded. Their repaired ultrawide unions are
`1284px` and `1924px`, both centred at `1280px`. Focused layout coverage
locks both cases.

Ordinary samples centred forest chunk `21`, mountain `77`, town `3`, coast
`400`, and industrial `129`. Their compact route positions were
`6754/24674/994/128034/41314`, desktop positions were
`7199/25119/1439/128479/41759`, and ultrawide positions were
`7839/25759/2079/129119/42399`. Boundary samples used chunks
`18/72/90/36/36` (mountain→forest, forest→mountain, coast→mountain,
industrial→forest, mountain→forest), with compact positions
`5794/23074/28834/11554/11554`, desktop
`6239/23519/29279/11999/11999`, and ultrawide
`6879/24159/29919/12639/12639`. DOM inspection confirmed the intended
ordinary region under the viewport centre and both named regions visible at
every boundary. These captures used compact day, desktop sunset, and
ultrawide night. Mounted route chunks stayed bounded at `30–34` compact,
`43–44` desktop, and `62–64` ultrawide.

The summit station lifecycle was also retained at desktop day: approach
`3278.944px`, decelerate `3457.266px`, platform/dwell/depart `3680px`, and
returned cruise `4067.197px`; every phase wait reported `passed: true`.
The 117 source captures remain in `/private/tmp/train-053-final/`. The
labelled, bottom-scene contact sheet is committed as
[`train-053-final-contact-sheet.png`](train-053-final-contact-sheet.png).
Visual inspection found crisp distinct regions and readable set pieces in all
palettes, with no repeated wallpaper, horizontal slab, monolithic station
wall, floating or pasted asset, opaque-window regression, collision, seam,
gap, clipped station, or metadata-only named composition. `borz errors
--json` was empty; console output contained only Vite connection and React
development messages. The new five-seed statistical suite covers bounded
route windows, determinism, regional/variant coverage, repetition distance,
set-piece visibility, depth order, binary solid alpha and emissive ownership,
palette contrast, and reduced-motion geometry; the focused convergence and
layout run passed all 92 tests. The complete frontend suite passed all 465
tests, the production TypeScript/Vite build passed, and repository-root
`make test` passed.

TRAIN-054 terrain-material cleanup run 2026-07-27: used the dedicated
`tmact-train-workitems` borz profile against only `127.0.0.1:5234`. Before
each route/viewport capture, the run confirmed HTTP 200, matched the plain
non-cache-busted `/src/components/TrainLayout.css` module to the new sparse
forest pixel marks, opened the exact proof URL, set the viewport, hard
reloaded, and asserted the actual `window.innerWidth` and
`window.innerHeight`. A live pane seat was selected before evidence capture.
Every scenery-only capture temporarily hid `.train-layout-inspection`,
`.train-world-debug-grid`, and `.train-time-toggle`, then removed the proof
style and verified their displays had returned to `flex`, `block`, and
`grid`.

Desktop day/night evidence used aurora forest `7199`, town `1439`, coast
`128479`, and the previous ordinary-mountain position `25119` at
`1280×800`. The hidden-train `.train-layout` rectangle measured
`y=572.921875`, height `184.078125`; each source image was cropped to the
185 physical pixels intersecting that exact band. Compact forest/town
day/night evidence reused `7199/1439` at an asserted `390×844`; its measured
rectangle was `y=623.75`, height `149.25`, cropped to the 150 intersecting
physical pixels. The twelve cropped source images remain in
`/private/tmp/train-054-terrain/`, and the labelled inspected contact sheet is
committed as
[`train-054-terrain-materials-contact-sheet.png`](train-054-terrain-materials-contact-sheet.png).

Every retained crop was opened individually at original resolution. The
forest soil, mountain rock, town ground, coast shore, and industrial fill
owners now read as opaque contour surfaces with sparse, bounded grass/leaf,
strata, seam, broken-shore, or engineered pixel marks; none retains a
terrain-owned fence, scaffold, diamond grid, or full-surface stripe. The
aurora `7199` frame also contains the known bridge lattice and the `25119`
frame contains the known tunnel bore. DOM inspection confirmed both are
non-terrain set-piece architecture while every visible `.train-terrain-base`
reported `background-image: none`; they were deliberately preserved for
TRAIN-059 and TRAIN-058 rather than hidden or changed in this material-only
item.

At default cruise speed, a desktop sample advanced route and near-layer
positions together from `1440.601px` to `1460.598px` over an 800 ms sample
while retaining 52 bounded terrain owners. `borz errors --json` was empty;
console output contained only Vite connection and React development
messages. The focused layout suite passed all 87 tests. The complete frontend
suite passed all 466 tests with one worker after the default parallel run
twice put the pre-existing long-route scenery test just over its 5 second
timeout; production TypeScript/Vite build and repository-root `make test`
both passed.

TRAIN-055 forest/mountain composition run 2026-07-27: used the dedicated
`tmact-train-workitems` borz profile against only `127.0.0.1:5234`. Each
forest/mountain and viewport pair independently confirmed HTTP 200, matched
the plain non-cache-busted source module to the current forest vegetation
owner and props-only near pool, opened the proof URL, set the viewport after
opening, hard reloaded, and asserted the exact `window.innerWidth` and
`window.innerHeight`. Forest positions were `6754/7199/7839px` and mountain
positions were `24674/25119/25759px` at compact `390×844`, desktop
`1280×800`, and ultrawide `2560×900`, respectively.

Every day, sunset, and night capture temporarily hid
`.train-layout-inspection`, `.train-world-debug-grid`, and
`.train-time-toggle`, then removed the proof style and verified their displays
returned to `flex`, `block`, and `grid`. The measured `.train-layout` rectangles
were `y=623.75`, height `149.25` compact; `y=572.921875`, height `184.078125`
desktop; and `y=672.921875`, height `184.078125` ultrawide. The physical crops
were therefore `390×150`, `1280×185`, and `2560×185`. All 18 source crops were
opened individually at original resolution and remain in
`/private/tmp/train-055-final/`; the labelled inspection sheet is committed as
[`train-055-forest-mountain-contact-sheet.png`](train-055-forest-mountain-contact-sheet.png).

Direct comparison with the corresponding TRAIN-053 contact-sheet tiles found
that ordinary forest no longer reads as mountains and terrain alone. The
foreground now has contour-anchored conifer, deciduous, hedgerow, reeds, and
bounded meadow/clearing families in all three palettes and widths, while the
generic near-track pool remains props-only and sparse. Mountain views replace
the equal-scale snowy-peak and orange-mesa row with separated ridge, rock-face,
alpine-vegetation, and negative-vista families. No retained crop introduced a
fence-like vegetation row, crosshatch, scaffold, diamond grid, or full-surface
stripe. The bridge lattice in the forest sample and tunnel body in the
mountain sample are the unchanged, non-regional traversal defects reserved for
TRAIN-059 and TRAIN-058.

At the default `24px/s` cruise speed, a desktop sample advanced route position
from `7205.361px` to `7225.401px` while the fixed consist stayed at `x=0`;
the bounded DOM remained at 45 route chunks and 18 ordinary scenery assets.
`borz errors --json` was empty. The focused scenery/layout/vegetation run
passed 127 tests, the single-worker complete frontend suite passed all 467
tests, the production TypeScript/Vite build passed, and repository-root
`make test` passed. The deliberately long 3,601-chunk determinism test now has
a focused 15-second timeout after the denser regional pools made its former
5-second default intermittently expire; its route range and assertions are
unchanged.

TRAIN-056 town/industrial normalization run 2026-07-27: used the dedicated
`tmact-train-workitems` borz profile against only `127.0.0.1:5234`. Every
route/viewport pair independently returned HTTP 200, matched the plain
non-cache-busted Vite source to the raster-fixture implementation, opened the
proof URL, set the viewport after opening, hard reloaded, and asserted the
actual inner dimensions. Town positions were `994/1439/2079px` at compact
`390×844`, desktop `1280×800`, and ultrawide `2560×900`. The industrial
comparison used the clear aurora crane/service district at `34417px` for
compact and desktop and `35057px` ultrawide.

Every day, sunset, and night capture temporarily hid
`.train-layout-inspection`, `.train-world-debug-grid`, and
`.train-time-toggle`, then removed the proof style and verified the live
displays returned to `flex` and `grid` (the debug grid was absent outside
debug mode). The measured `.train-layout` rectangles were `y=660.75`,
height `149.25` compact; `y=572.921875`, height `184.078125` desktop; and
`y=672.921875`, height `184.078125` ultrawide. The physical crops were
therefore `390×150`, `1280×185`, and `2560×185`. All 18 crops were opened
individually at original resolution and remain in
`/private/tmp/train-056-final/`; the labelled inspected sheet is committed as
[`train-056-town-industrial-contact-sheet.png`](train-056-town-industrial-contact-sheet.png).

Direct inspection found that the crude CSS townhouse, shop, shed, and gantry
boxes no longer sit beside the detailed raster church, rowhouse, cottage, or
warehouse art. Building-shaped fixtures now reuse the same native-1x raster
family, share a human-scale module and shallow-three-quarter perspective, and
sit on explicit contour-owned foundations. Streets, yards, fences, utility
corridors, stacks, tanks, and service pipes connect the structures while
retaining deliberate gaps. Day captures contained no emissive overlays;
sunset/night window masks and the industrial beacon stayed attached to their
owners. The first compact industrial sample was rejected because it showed
only a utility corridor, and a later night sample was rejected because the
old gantry beacon coordinate floated above the raster; both were corrected
and recaptured before acceptance.

At ultrawide width, the compositor necessarily retains the previously known
bridge at the left edge and station campus at the right edge of the industrial
frame. A deterministic multi-seed search found no industrial ultrawide window
without projected set pieces; candidates with fewer projections moved a
station or town-edge composition across the centre and were visually worse.
The edge defects remain visible and unmodified for TRAIN-059 and TRAIN-060
rather than being hidden by proof CSS. The ordinary industrial grammar remains
readable across the left/centre of the retained comparison, and its raster
assets do not mix pixel density or float above their foundations.

At the default `24px/s` cruise speed, a desktop sample advanced route position
from `34448.647px` to `34469.448px` over 850 ms and midground position from
`18946.756px` to `18958.196px`; the fixed consist remained at `x=0` and the
bounded DOM stayed at 45 route chunks. The focused scenery/layout run passed
all 123 tests, the single-worker complete frontend suite passed all 468 tests,
the production TypeScript/Vite build passed, and repository-root `make test`
passed.

TRAIN-057 coast grounding and reveal run 2026-07-27: used only the dedicated
`tmact-train-workitems` borz profile and `127.0.0.1:5234`. For ordinary coast
seed `train-053-aurora` at route position `128479` and focused coast-reveal
seed `train-053-orchard`, occurrence `0`, every scene/viewport pair
independently returned HTTP 200, matched the plain non-cache-busted
`TrainLayout.tsx` Vite module to the current ownership metadata, opened the
URL, set the viewport after opening, hard reloaded, and asserted the actual
inner dimensions.

The measured `.train-layout` rectangles were `y=660.75`, height `149.25` for
the compact ordinary view and `y=623.75`, height `149.25` for compact focused
reveal; desktop was `y=572.921875`, height `184.078125`; ultrawide was
`y=672.921875`, height `184.078125`. Each day/night pair retained both a
train-visible contact crop and a scenery-only crop. Scenery captures hid the
live `.train-layout-inspection` tree plus debug/time controls, then restored
every original inline style and verified no capture marker remained. All 24
crops were opened individually at original resolution and remain in
`/tmp/train057-final/`; the labelled inspected sheet is committed as
[`train-057-coast-contact-sheet.png`](train-057-coast-contact-sheet.png).

Direct pixel inspection found continuous water with opaque dry shoreline
shelves under every visible cottage, lighthouse, post, pier, and vegetation
asset; no building base terminates inside water. Boats and buoys use a
separate waterline owner, and lighthouse reflection geometry remains clipped
to that same water plane. The exact `128479` route includes its deterministic
coast transition, so an additional ordinary coast interior at `130000` was
inspected in day and night to expose the full dry-shelf contact rather than
letting the fixed train or transition conceal it.

The first compact focused-reveal capture exposed town fixtures inside chunks
already marked `data-scenery-reserved="projected-set-piece"`. This was
rejected: regional grounds, forest details, coast compositions, and built
fixtures now all honor the same scenery reservation. Fresh compact, desktop,
and ultrawide captures then showed the reveal as an enclosed-land-to-broad-
water opening with zero reserved-fixture leakage, while far, midground, and
near owned only water/horizon, shore framing, and track foreground
respectively. Ordinary coast retained near-shore detail and was visibly
distinct from the sparse focused reveal.

At default `24px/s` speed, the focused reveal remained in full running motion
and advanced route position from `128323.187px` to `128333.999px` over 450 ms
(`10.812px`), with no leftover capture styles. The focused layout/scenery run
passed all 123 tests, the single-worker complete frontend suite passed all 468
tests, the production TypeScript/Vite build passed, and repository-root
`make test` passed.

TRAIN-058 tunnel traversal run 2026-07-27: used only the dedicated
`tmact-train-workitems` borz profile and `127.0.0.1:5234`. The cascade tunnel
occurrences `0/1` supplied stepped-arch variant `1` and round-arch variant `0`.
For every entry/body/exit and palette/viewport pair, the run independently
returned HTTP 200, matched the plain non-cache-busted `TrainLayout.tsx` Vite
module to the current tunnel ownership source, opened the exact route-position
URL, set the viewport after opening, hard reloaded, and asserted the actual
inner dimensions plus the centred DOM role and variant.

The measured `.train-layout` rectangles were `y=623.75`, height `149.25` at
compact `390×844`; `y=572.921875`, height `184.078125` at desktop `1280×800`;
and `y=672.921875`, height `184.078125` at ultrawide `2560×900`. Entry/body/
exit positions used the variant focus centre minus `581.818px`, at centre,
and plus `581.818px`. Compact and desktop retained day/night train-hidden and
train-visible sequences for both variants; ultrawide retained one day
train-hidden/train-visible sequence per variant. The resulting 60 real-band
crops were inspected at native resolution through ten entry/body/exit sequence
strips in `/tmp/train-058-final/`. The labelled committed matrix is
[`train-058-tunnel-contact-sheet.png`](train-058-tunnel-contact-sheet.png).

Direct pixel inspection rejected the initial implementation because the body
still read as a broad dark face and its lining blended into the mountain.
The retained result instead shows one continuous rail-aligned passage with
opaque enclosing rock, visible crown and side lining, bounded bore shading,
a readable floor, and low trackside shoulders. The primary midground layer
owns the rock, portal, and one bore per segment; the supporting near layer owns
only rail-contact geometry and never paints a second opening. Round and stepped
silhouettes remain visibly distinct in day and night. No retained crop shows a
flat viewport-wide black rectangle, detached portal, duplicate side bore, or
rail line below the opening; the fixed consist remains bright and readable.

At default `24px/s`, a desktop sample advanced route position by `15.573px`
over 650ms while the fixed consist stayed at `x=0`. The live focused
composition contained three primary and three supporting segments, exactly
three primary openings, zero supporting openings, zero collision exclusions,
and no browser errors or leftover capture style. The focused TrainLayout run
passed all 88 tests, the single-worker complete frontend suite passed all 469
tests, the production TypeScript/Vite build passed, and repository-root
`make test` passed.

TRAIN-059 bridge traversal run 2026-07-27: used only the dedicated
`tmact-train-workitems` borz profile and `127.0.0.1:5234`. Aurora bridge
occurrence `2` supplied river pony-truss variant `0`; occurrence `0` supplied
stone-parapet gorge variant `1`. Every retained route-position/viewport pair
independently returned HTTP 200, matched the plain non-cache-busted
`TrainLayout.tsx` Vite module to the new bridge ownership metadata, opened the
exact URL, set the viewport after opening, hard reloaded, and asserted the
actual inner dimensions plus the centred entry/body/exit role and variant.

The measured `.train-layout` rectangles were `y=623.75`, height `149.25` at
compact `390×844`; `y=572.921875`, height `184.078125` at desktop
`1280×800`; and `y=672.921875`, height `184.078125` at ultrawide
`2560×900`. Compact and desktop retained day/night train-hidden and
train-visible entry/span/exit sequences for both variants. The required one
ultrawide sequence retained variant `0` entry/span/exit in day with both
visibility states. All 54 real-band crops were opened individually at original
resolution in `/tmp/train-059-final/`; the labelled inspected matrix is
committed as
[`train-059-bridge-contact-sheet.png`](train-059-bridge-contact-sheet.png).

Direct pixel inspection rejected the initial rebuild twice: the first stone
variant was still a thick pale wall with supports crossing it, and the second
still used the day control-surface token for a bright gorge. The retained
result gives variant `0` a readable river beneath a low local pony truss and
variant `1` a dark lowered gorge beneath a thin stone parapet and short piers.
Entry and exit use sloped owned approaches; body segments carry the crossing,
continuous deck, and supports. The primary midground layer is the only owner
of crossing, deck, supports, and structure; the near supporting layer owns one
low track-contact edge per segment and never paints a second bridge.

Across the retained sequences, truss members stay in the lower bridge band and
do not cover the mountain field or recreate terrain crosshatch. The compact
exit crops show the train back on land after the bridge has moved beyond the
narrow frame; their preceding entry/body crops preserve the complete crossing
transition. Desktop and ultrawide exit crops retain the departing structure
at the far side. No inspected crop shows a viewport-wide lattice, duplicated
truss or deck, missing crossing subject, support above the bridge structure,
or wheels floating above/below the rail/deck line. A desktop variant `1` exit
window legitimately collision-excluded an incompatible station id while
retaining all four bridge segments; the bridge id itself was never excluded.

At default `24px/s`, a desktop sample advanced route position from
`36491.589px` to `36507.232px` over 650ms (`15.643px`). The fixed consist
remained at `x=0`, wheel rotation changed from `-112.670deg` to
`-212.257deg`, and the live composition retained four primary plus four
supporting segments with no browser errors. The focused bridge/terrain run
passed all 93 tests, the single-worker complete frontend suite passed all 470
tests, the production TypeScript/Vite build passed, and repository-root
`make test` passed.

TRAIN-060 station campus run 2026-07-27: used only the dedicated
`tmact-train-workitems` borz profile and `127.0.0.1:5234`, with canonical pane
`%46` (`tmact-train-workitems:0.0`). Every requested viewport first returned
HTTP 200, matched the plain non-cache-busted `TrainLayout.tsx` Vite module to
the current station source, opened the route-position URL, set the viewport
after opening, hard reloaded, and asserted the actual inner dimensions before
capture.

The measured `.train-layout` rectangles were `y=660.75`, height `149.25` at
compact `390×844`; `y=572.921875`, height `184.078125` at desktop
`1280×800`; and `y=672.921875`, height `184.078125` at ultrawide
`2560×900`. Entry/centre/exit route positions were `2915/3715/4515`,
`3360/4160/4960`, and `4000/4800/5600` respectively. Approach, platform,
dwell, and departure were also reproduced through the actual station lifecycle.
Each of the seven states was retained in day and night with the train both
hidden and visible, producing 84 measured real-band crops under
`/private/tmp/train-060-proof/`. All retained pixels were opened at original
resolution through the six viewport/palette contact strips. The labelled
committed comparison is
[`train-060-station-contact-sheet.png`](train-060-station-contact-sheet.png).

Direct pixel inspection shows three separated short station masses, open
platform stretches, local supported shelters, explicit entrances, furniture,
and scenery-visible negative space. Entry and exit remain square and complete;
there is no broad featureless wall, floating canopy, clipped end, or return of
the previous diagonal cut. Train-visible captures confirm that the fixed
consist naturally overlaps the opaque platform edge while doors, windows,
canopies, and station identity remain readable in compact, desktop, and
ultrawide views.

The final live composition contained three buildings, four canopies, seven
canopy supports, four lamps, six openings, and three entrances, with no browser
errors or leftover capture styles. Regression coverage locks campus mass roles,
negative spaces, segment roles, fixed-train/platform overlap, sparse fixtures,
day/sunset/night illumination, and approach/platform/dwell/depart lifecycle
stability. The focused station run passed all 101 tests, the complete frontend
suite passed all 471 tests, the production TypeScript/Vite build passed, and
repository-root `make test` passed.

TRAIN-061 adversarial visual audit run 2026-07-27: audited the converged train
scene as a red-team pass rather than assuming the preceding item proofs were
still sufficient. The run used only the dedicated `tmact-train-workitems`
borz profile and `127.0.0.1:5234`. Before trusting each compact `390×844`,
desktop `1280×800`, and ultrawide `2560×900` pass, it independently received
HTTP 200, fetched the plain non-cache-busted Vite module and matched it to the
current traversal ownership source, opened the page, set the viewport after
opening, hard reloaded, and asserted the actual inner dimensions.

The audit covered all five deterministic seeds from TRAIN-053, all five
ordinary regions, both variants of bridge, tunnel, town-edge, coast-reveal,
and station, plus station approach/decelerate/platform/dwell/depart/cruise
states. It also searched the deterministic routes and inspected all seven
observed region boundary families: mountain→forest, coast→mountain,
town→mountain, industrial→forest, industrial→mountain, town→forest, and
forest→mountain. Default-speed motion remained live: a desktop sample advanced
route and near-layer positions together from `7202.955px` to `7223.799px`
over 850ms, with the fixed train still separated from the moving world.

The measured `.train-layout` rectangles were `y=623.75`, height `149.25`
compact; `y=572.921875`, height `184.078125` desktop; and `y=672.921875`,
height `184.078125` ultrawide. Scenery captures hid the train and inspection
controls, then restored the live DOM; train-visible bridge, tunnel, station,
platform, shore, and boundary captures separately checked contact geometry.
Every retained compact, desktop, ultrawide, phase, boundary, and historical
comparison crop was opened at original resolution and judged on its pixels.
The labelled committed comparison places the real TRAIN-053 failure bands
beside the corresponding final bands:
[`train-061-before-after-contact-sheet.png`](train-061-before-after-contact-sheet.png).

The red-team pass found three remaining defects instead of broadening scope.
First, bridge variant 0 exposed too little river and variant 1 still read as a
horizontal wall rather than a gorge. The crossing is now deeper, the river and
dark gorge surfaces are locally visible, and supports terminate below a thin
deck/parapet. Second, all three tunnel segment apertures were visible at once,
recreating the wide multi-bore defect. Live route position now selects the
entry, body, or exit aperture, hides inactive portal geometry, and orients end
portals inward while preserving one bounded bore. Third, a compact forest
bridge reservation could leave the focused crossing almost vegetation-free.
Existing conifer and hedgerow raster assets now frame bridge approaches and
root body vegetation on the gorge rim without creating a viewport lattice.

Fresh train-hidden and train-visible captures confirmed that river, gorge,
deck, wheel/rail, bore, portal, forest rim, platform, and shoreline contacts
remain possible at all retained widths. The station continues to preserve
short masses and negative space throughout the live stop lifecycle; town and
industrial structures retain one pixel-density family and contour-owned
foundations; coast fixtures remain separated into dry-land and waterline
owners; coast reveal remains visibly sparser than ordinary coast. No retained
band shows terrain crosshatch, a viewport-wide bridge lattice, three tunnel
openings, a station wall, a floating shoreline fixture, or a deterministic
boundary seam.

Two proof failures were treated as diagnostics and discarded: an early station
lifecycle script overshot the requested phases, and the initial compact forest
bridge result remained too sparse. The station phases were reproduced from
the live state machine, and the bridge was fixed and recaptured before
acceptance. Browser error inspection was empty; console output contained only
Vite connection/HMR and React development notices. Focused layout and visual
convergence coverage passed all 97 tests, including new locks for regional
rhythm, pixel-density/ground ownership, coast contact, station negative space,
single active tunnel aperture, bridge crossing depth, and forest rim framing.
The single-worker complete frontend suite passed all 472 tests, the production
TypeScript/Vite build passed, and repository-root `make test` passed.

## Notes Template

```text
Date:
Command:
Target:
Result:
Follow-up:
```
