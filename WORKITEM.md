# Train Layout — Infinite Journey Background Work Items

This queue turns the train-theme background plan into twenty dependency-ordered,
independently verifiable work items. The finished scene must make the fixed
train feel as if it is travelling forever through coherent regions, support
day/sunset/night presentation, remain deterministic enough to test, and allow a
future station stop without growing the DOM or memory usage over time.

## Worker contract

- Work from this repository's root and read `AGENTS.md`, `CLAUDE.md`, and this
  file before starting an item.
- Select only the first unchecked item. Never start a second item in the same
  cycle.
- The existing train layout, locomotive, carriage, track, seat, and character
  work is the baseline. Do not redraw or restructure those assets unless the
  selected item explicitly requires integration changes.
- Before automation starts, the current train-layout baseline should be
  committed. If the worktree contains pre-existing changes, never reset, clean,
  stash, overwrite, or discard them; report the exact paths as a blocker.
- Keep the train and its horizontal inspection/overflow behavior independent
  from world movement. The train stays visually fixed while the scenery moves
  from left to right because the locomotive faces left.
- Prefer a small deterministic route engine and reusable scene assets over one
  very wide image, unbounded DOM nodes, or uncontrolled per-frame randomness.
- Use seeded randomness for route generation. The same seed, route position,
  viewport, and time-of-day input must reproduce the same scene.
- Respect `prefers-reduced-motion`, tab visibility, viewport resizing, and the
  existing theme/settings architecture.
- Add focused Vitest coverage with every behavioral item. Use fake time and
  deterministic seeds where animation or randomness is involved.
- For visual work, verify with `borz` at compact and wide viewport sizes. Check
  that there are no gaps, jumps, obvious repeated patterns, or collisions that
  obscure the train.
- Run targeted tests during implementation. Before completing an item, run the
  frontend test suite and build; run `make test` when shared application
  behavior or Go-embedded web output is affected.
- Complete each item atomically: implementation, tests, documentation/debug
  affordances, and exactly that item's checkbox belong in one commit. Use a
  concise imperative commit subject and do not push.
- If blocked, do not check the item or commit partial work. If all items are
  checked, make no changes and report queue completion.

## Queue

- [x] **TRAIN-001 — Establish the independent moving-world viewport.**
  Add a background/world layer behind `TrainLayout` whose clipping, dimensions,
  and animation state do not depend on the horizontal overflow used to inspect
  the locomotive and carriages. Define one route-position value and make the
  scenery move left-to-right while the train remains fixed. Establish explicit
  z-index and pointer-event boundaries so the world never blocks train or
  locomotive interactions. Include a temporary diagnostic grid/marker to prove
  motion direction and independence, then keep it available only through a
  development flag. Cover mounting, layering, direction, and train-scroll
  independence with focused tests.

- [x] **TRAIN-002 — Build a deterministic infinite route-chunk engine.**
  Divide the journey into fixed-width logical `RouteChunk`s generated from a
  versioned seed and integer chunk index. Maintain only the visible chunks plus
  bounded overscan in a recyclable ring/window; travelling for a long time must
  not continually add DOM nodes or retained route objects. Handle forward
  movement and viewport resizing without blank seams or regenerating already
  visible chunks differently. Expose lightweight development diagnostics for
  seed, route position, chunk indices, and mounted chunk count. Test stable
  generation, recycling, long-distance bounds, resize behavior, and seed
  variation.

- [x] **TRAIN-003 — Add the five-layer parallax scene renderer.**
  Render each route through explicit layers: sky, ultra-far silhouette, far
  terrain, midground scenery, and near/foreground objects. Start with code-native
  placeholder shapes and assign restrained speed ratios around `0`, `0.1`,
  `0.25`, `0.55`, and `1.0`, keeping the train on its fixed layer. Adjacent
  chunks and parallax layers must overlap safely enough to avoid hairline gaps
  at fractional pixels. Pause or simplify motion for reduced-motion users.
  Test transform calculations, layer ordering, seam overlap, and reduced-motion
  behavior; visually verify both compact and ultrawide screens.

- [x] **TRAIN-004 — Produce the reusable pixel-art scenery asset kit.**
  Create transparent, scale-consistent assets matching the existing train's
  pixel-art perspective and palette. The minimum kit is: three cloud forms,
  three far mountain/terrain silhouettes, six tree/vegetation variants, six
  town or industrial building variants, one bridge set, one coast/sea set, and
  three near-track props such as poles, signs, or fences. Keep distant assets
  simple and low-contrast so they do not compete with passengers; reserve the
  strongest detail and contrast for near objects. Record intended layer,
  anchor, safe scale range, and day/night treatment in an asset manifest.
  Replace TRAIN-003 placeholders and verify transparent edges, pixel scaling,
  anchors, and seamless terrain joins in the live layout.

- [x] **TRAIN-005 — Generate coherent regions with route grammar.**
  Introduce region profiles for at least forest, mountain/foothill, town,
  coast, and industrial outskirts. A region should last roughly 6–12 chunks and
  constrain which assets, densities, terrain, and landmarks may appear; do not
  scatter every object type uniformly. Add weighted transitions so neighboring
  regions form plausible journeys, for example forest → foothills → suburb →
  town or town → coast. Enforce per-layer spacing, collision, maximum density,
  landmark limits, and cooldown/deweighting for recently used variants. Test
  allowed transitions, deterministic output, density bounds, spacing, and
  repeat avoidance across a long seeded route.

- [x] **TRAIN-006 — Add day, sunset, and night as palette states.**
  Make time of day independent from route geometry: changing between day,
  sunset, and night must recolor/crossfade the same visible terrain rather than
  regenerate its objects or jump the route. Define shared CSS palette tokens
  for sky, haze, silhouettes, surfaces, water, and foreground contrast. Add
  separate emissive overlays for stars, moon, windows, streetlights, station
  lamps, signals, and water reflections so night is readable without baking
  three complete copies of every asset. Initially follow the office layout's
  local-time selection and manual theme override. Test selection rules, manual
  override, stable geometry, transition state, and accessible contrast.

- [x] **TRAIN-007 — Make continuous motion smooth, bounded, and lifecycle-safe.**
  Drive route position from elapsed time rather than frame count, with one
  animation owner and configurable cruise speed. Clamp large elapsed-time jumps,
  suspend expensive updates while the document is hidden, resume without a
  scenery leap, and avoid React rerendering the whole train every frame. Ensure
  window resizing, route recycling, lazy asset loading, and manual train
  scrolling cannot produce a visible gap. Add a reduced-motion mode that keeps
  a complete static scene and advances only through restrained, infrequent
  steps when needed. Test fake-clock progression, pause/resume, throttled-frame
  recovery, cleanup, and bounded render/mount counts.

- [x] **TRAIN-008 — Add landmarks, bridges, and transition set pieces.**
  Promote bridges, tunnels/cuttings, coastline reveals, town edges, and other
  large compositions to deterministic multi-chunk set pieces rather than
  ordinary random props. Reserve their chunk spans before filling smaller
  objects, provide entry/body/exit pieces, and prevent incompatible landmarks
  from overlapping. Give set pieces restrained foreground occlusion so the
  carriage and passengers remain legible. Add at least one complete bridge
  traversal and one coast or tunnel transition. Test reservations, chunk-boundary
  continuity, incompatibility rules, and deterministic entry/body/exit output;
  visually inspect the entire traversal at cruise speed.

- [x] **TRAIN-009 — Implement station approach, stop, and departure.**
  Add a station as a scheduled route event with the state machine `cruise →
  approach/signals → decelerate → platform → dwell → depart/accelerate →
  cruise`. Reserve the station's multi-chunk span, introduce signals and
  platform/building assets, and expose explicit speed targets rather than
  coupling station logic to animation frames. During dwell, positional scenery
  must stop while ambient details such as lights, clouds, steam, or subtle
  passenger effects may continue. Prevent another station from appearing until
  a configurable minimum journey distance has passed. Provide a deterministic
  development trigger to reach/leave a station quickly. Test every state
  transition, speed curve boundary, dwell timing, cooldown, tab pause/resume,
  and route continuity after departure.

- [x] **TRAIN-010 — Integrate, tune, and harden the complete journey.**
  Remove temporary placeholders, make the background load only for the train
  theme, and ensure all animation/resources clean up when switching themes.
  Tune scale, speed, density, haze, palette transitions, and foreground
  occlusion across compact mobile-like widths, normal desktop, and ultrawide
  screens. Add an end-to-end deterministic journey test covering multiple
  regions, a time-of-day change, a landmark, a station stop, departure, resize,
  theme switch, and remount. Run frontend tests, build, and `make test`, then
  perform a sustained `borz` smoke run checking constant DOM bounds, no blank
  seams, stable train controls, acceptable CPU behavior, and readable day,
  sunset, and night scenes. Document the route seed/debug controls and final
  manual verification cases.

- [x] **TRAIN-011 — Keep reduced-motion station timing on wall-clock time.**
  Decouple the deliberately infrequent reduced-motion route steps from the
  station's non-positional timing. With `prefers-reduced-motion: reduce`, keep
  cruise/approach scenery movement restrained and infrequent, but make the
  250ms platform-settle phase and four-second dwell complete on real elapsed
  wall-clock time rather than stretching to roughly 90 seconds and 20 minutes.
  Preserve visibility suspension semantics so hidden time cannot cause a route
  jump or silently skip an observable station phase. Add fake-timer coverage
  for reduced-motion approach, platform, dwell, departure, visibility
  suspend/resume, cleanup, and unchanged bounded route stepping. Run the
  frontend suite, build, and `make test`.

- [x] **TRAIN-012 — Make stations visually legible without obscuring the train.**
  Recompose or retune the station platform, canopy/building silhouette, signals,
  lamps, and ambient steam so an approach, stop, and departure read clearly as
  a station at 390px compact, normal desktop, and 1920px-or-wider viewports.
  Do not move the complete world above the train, block controls, cover
  passengers, or weaken horizontal train inspection. Preserve deterministic
  station spans and the existing state machine. Add focused structural/style
  assertions, then use the shared Vite server and `borz` to capture and inspect
  approach/platform/dwell states at compact and wide sizes. Run the frontend
  suite, build, and `make test`.

- [x] **TRAIN-013 — Tune station discovery and repeat cadence.**
  Shorten the default seed's initial journey so a normal 12px/s session begins
  its first station approach within roughly three to five minutes, and tune
  later station spacing to roughly six to nine minutes without permitting
  back-to-back stations or violating the configured minimum journey distance.
  Keep scheduling deterministic, aligned to complete multi-chunk regions, and
  configurable for callers that need longer journeys. Add exact default-seed
  timing assertions, multi-seed bounds, cooldown tests, and continuity coverage
  after departure. Verify the development triggers remain unchanged. Run the
  frontend suite, build, and `make test`.

- [x] **TRAIN-014 — Advance clock-driven palettes at local-time boundaries.**
  Give the train layout an owned, cleanup-safe clock update so an open page
  automatically crosses day → sunset at 17:00 and sunset → night at 18:30
  without relying on pane traffic, resizing, or another unrelated React render.
  Schedule efficiently at the next meaningful boundary rather than rerendering
  every animation frame. Manual palette overrides must remain stable and route
  geometry/position must not change. Add fake-system-time tests for both
  boundaries, manual override isolation, timer cleanup, and remount behavior.
  Run the frontend suite, build, and `make test`.

- [x] **TRAIN-015 — Make browser smoke checks resistant to false viewport passes.**
  Update the train smoke procedure to require this exact order for every size:
  open/reopen the shared port-5234 page with `borz`, set the viewport, hard
  reload, then assert `window.innerWidth` and `window.innerHeight` before
  trusting layout or screenshots. Record the plain non-cache-busted Vite module
  freshness check and make compact, desktop, and ultrawide evidence explicit.
  Re-run the documented station and bounded-DOM checks with `borz`, record the
  date and results without private pane data, run `rtk git diff --check`, and
  keep this item documentation-only unless a reproducible product defect is
  found; if one is found, leave this item unchecked and report it instead of
  expanding scope.

- [x] **TRAIN-016 — Move the railway track into the travelling world.**
  Remove the track strip from the horizontally scrollable fixed-train
  inspection scene and render it as a dedicated bounded world element behind
  the wheels. The track and sleepers must follow route/world position at the
  near-layer speed, travelling with the scenery rather than with the locomotive
  and carriages; they must stop during station dwell, suspend while hidden, and
  retain the restrained stepped behavior under reduced motion. Horizontal train
  inspection must never translate, resize, duplicate, or reveal an edge in the
  track. Preserve the existing wheel alignment, track height, fixed consist,
  pointer boundaries, station continuity, and compact-to-ultrawide coverage.
  Add focused tests for DOM ownership, route transform/direction, inspection
  independence, dwell/visibility/reduced-motion lifecycle, cleanup, and bounded
  rendering. Validate with `borz` at compact, desktop, and ultrawide sizes using
  the exact viewport procedure documented by TRAIN-015, then run the frontend
  suite, build, and `make test`.

- [x] **TRAIN-017 — Cut transparent scenery windows into the carriage sprite.**
  Edit
  `internal/web/frontend/src/assets/train-theme/sprites/train-carriage-empty-v2.png`
  so the two large upper/lower passenger-window interiors have genuine alpha
  transparency and reveal the moving world behind the carriage. Follow the
  imagegen skill in built-in edit mode: inspect the local edit target first,
  preserve the existing pixel-art carriage body, window frames, lamps,
  staircase, couplers, wheels, palette, silhouette, dimensions, and alignment,
  and change only the pane interiors. Use the skill's flat chroma-key plus local
  removal workflow if needed; do not silently switch to the CLI/native-alpha
  fallback. The final committed sprite must remain exactly 821×383 RGBA, retain
  transparent exterior corners, contain transparent pane centers without
  chroma fringe, and keep the frames/body opaque. Add deterministic asset checks
  for dimensions and representative alpha regions. With the world moving
  independently behind it, use `borz` to verify scenery is visibly readable
  through both empty and occupied carriage windows in day, sunset, and night at
  compact and wide sizes, without weakening seat clicks, focus states, or text
  contrast. Run the frontend suite, build, and `make test`.

- [x] **TRAIN-018 — Match the office layout height and reveal more sky.**
  Make the train and office pane-switcher layouts resolve to the same outer
  height at both desktop and the existing ≤760px compact breakpoint. Define one
  shared height contract/token or shared calculation so the two layouts cannot
  silently drift apart again; do not duplicate unrelated office internals into
  the train component. Use the train's added vertical space for atmosphere and
  sky above the consist: keep the track/wheel baseline, fixed-train behavior,
  carriage proportions, controls, and horizontal inspection position stable
  rather than stretching or vertically centering the train artwork. Preserve
  the office scene's current appearance and height. Add focused contract tests,
  then use `borz` at 390×844, normal desktop, and ultrawide sizes to assert the
  two rendered layout heights are equal and the train gains visible sky without
  clipping or blank scenery. Run the frontend suite, build, and `make test`.

- [ ] **TRAIN-019 — Scale the train artwork to 90% while enlarging hit targets.**
  Reduce the visible locomotive, carriages, wheels, seats, and passenger artwork
  to exactly 90% of their post-TRAIN-018 size, bottom-anchored to the unchanged
  moving-world track baseline. Do not scale the world, track, layout height,
  menus, labels, focus rings, or pointer target geometry. Compensate for the
  smaller artwork by making every passenger seat target at least 44×44 CSS
  pixels at compact and desktop sizes, with sensible non-overlapping hit slop
  that still selects the intended passenger near each target edge. Keep the
  locomotive overflow trigger comfortably tappable, recalculate filler
  carriages so the consist still covers wide viewports, and preserve keyboard
  navigation, visible focus, horizontal inspection, and selected/stale states.
  Add focused scale, measurement, packing, pointer-edge, and keyboard tests.
  Validate empty/occupied compact, desktop, and ultrawide layouts with `borz`,
  then run the frontend suite, build, and `make test`.

- [ ] **TRAIN-020 — Give selected passengers a Diablo-II-style golden set aura.**
  Replace the selected passenger's current cyan sprite outline with a restrained
  golden equipped-set treatment inspired by Diablo II: a crisp pale-gold inner
  silhouette, warm amber outer glow, and subtle localized aura behind or below
  the character. Apply it only to the selected passenger, not the carriage,
  empty seats, hover state, or keyboard focus ring. It must not shift layout,
  blur character identity, cover adjacent targets, reduce labels/controls
  contrast, or depend on a raster duplicate. A slow subtle shimmer is allowed
  for full motion; `prefers-reduced-motion` must use an equally legible static
  gold treatment with no pulsing. Add focused selection/unselection, focus,
  stale-state, exclusivity, and reduced-motion coverage. Use `borz` to inspect
  the effect in day, sunset, and night palettes with compact and wide occupied
  carriages, then run the frontend suite, build, and `make test`.
