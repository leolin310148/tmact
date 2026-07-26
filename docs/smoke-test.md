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

## Notes Template

```text
Date:
Command:
Target:
Result:
Follow-up:
```
