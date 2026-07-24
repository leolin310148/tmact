# Train Layout — Infinite Journey Background Work Items

This queue turns the train-theme background plan into ten dependency-ordered,
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

- [ ] **TRAIN-003 — Add the five-layer parallax scene renderer.**
  Render each route through explicit layers: sky, ultra-far silhouette, far
  terrain, midground scenery, and near/foreground objects. Start with code-native
  placeholder shapes and assign restrained speed ratios around `0`, `0.1`,
  `0.25`, `0.55`, and `1.0`, keeping the train on its fixed layer. Adjacent
  chunks and parallax layers must overlap safely enough to avoid hairline gaps
  at fractional pixels. Pause or simplify motion for reduced-motion users.
  Test transform calculations, layer ordering, seam overlap, and reduced-motion
  behavior; visually verify both compact and ultrawide screens.

- [ ] **TRAIN-004 — Produce the reusable pixel-art scenery asset kit.**
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

- [ ] **TRAIN-005 — Generate coherent regions with route grammar.**
  Introduce region profiles for at least forest, mountain/foothill, town,
  coast, and industrial outskirts. A region should last roughly 6–12 chunks and
  constrain which assets, densities, terrain, and landmarks may appear; do not
  scatter every object type uniformly. Add weighted transitions so neighboring
  regions form plausible journeys, for example forest → foothills → suburb →
  town or town → coast. Enforce per-layer spacing, collision, maximum density,
  landmark limits, and cooldown/deweighting for recently used variants. Test
  allowed transitions, deterministic output, density bounds, spacing, and
  repeat avoidance across a long seeded route.

- [ ] **TRAIN-006 — Add day, sunset, and night as palette states.**
  Make time of day independent from route geometry: changing between day,
  sunset, and night must recolor/crossfade the same visible terrain rather than
  regenerate its objects or jump the route. Define shared CSS palette tokens
  for sky, haze, silhouettes, surfaces, water, and foreground contrast. Add
  separate emissive overlays for stars, moon, windows, streetlights, station
  lamps, signals, and water reflections so night is readable without baking
  three complete copies of every asset. Initially follow the office layout's
  local-time selection and manual theme override. Test selection rules, manual
  override, stable geometry, transition state, and accessible contrast.

- [ ] **TRAIN-007 — Make continuous motion smooth, bounded, and lifecycle-safe.**
  Drive route position from elapsed time rather than frame count, with one
  animation owner and configurable cruise speed. Clamp large elapsed-time jumps,
  suspend expensive updates while the document is hidden, resume without a
  scenery leap, and avoid React rerendering the whole train every frame. Ensure
  window resizing, route recycling, lazy asset loading, and manual train
  scrolling cannot produce a visible gap. Add a reduced-motion mode that keeps
  a complete static scene and advances only through restrained, infrequent
  steps when needed. Test fake-clock progression, pause/resume, throttled-frame
  recovery, cleanup, and bounded render/mount counts.

- [ ] **TRAIN-008 — Add landmarks, bridges, and transition set pieces.**
  Promote bridges, tunnels/cuttings, coastline reveals, town edges, and other
  large compositions to deterministic multi-chunk set pieces rather than
  ordinary random props. Reserve their chunk spans before filling smaller
  objects, provide entry/body/exit pieces, and prevent incompatible landmarks
  from overlapping. Give set pieces restrained foreground occlusion so the
  carriage and passengers remain legible. Add at least one complete bridge
  traversal and one coast or tunnel transition. Test reservations, chunk-boundary
  continuity, incompatibility rules, and deterministic entry/body/exit output;
  visually inspect the entire traversal at cruise speed.

- [ ] **TRAIN-009 — Implement station approach, stop, and departure.**
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

- [ ] **TRAIN-010 — Integrate, tune, and harden the complete journey.**
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
