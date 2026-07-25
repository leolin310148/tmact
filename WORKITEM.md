# Train Layout — Scenery Redesign Work Items

Completed TRAIN-001–026 are archived in
[docs/archive/train-layout-workitems-001-026.md](docs/archive/train-layout-workitems-001-026.md).

This active queue contains twelve dependency-ordered, independently verifiable
work items that rebuild scenery compositing and daylight art before expanding
regional day/night richness. The fixed train, route engine, station behavior,
and safety properties established by the archived queue remain the baseline.

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
- Treat daylight-neutral opaque pixel art as the canonical scenery source.
  Create depth with value, chroma, detail, scale, and a separate atmospheric
  veil—not by making solid terrain, vegetation, buildings, or props
  translucent. Apart from intentionally soft clouds, steam, glow, or
  reflections, visible sprite interiors should remain opaque.
- Keep illumination separate from solid art. Building windows, lamps, signals,
  fires, and reflections may use geometry-aligned emissive masks or overlays,
  but must not be permanently baked on in daylight or float detached from the
  object that owns them.
- Preserve a coherent pixel-art direction across all generated assets:
  transparent background, crisp nearest-neighbor edges, consistent perspective,
  restrained detail by depth, explicit anchors, and no photographic blur,
  antialiased halo, matte fringe, or semi-transparent solid fill.
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

- [x] **TRAIN-027 — Replace translucent depth stacking with an opaque scenery compositor.**
  Remove the compounded layer-and-sprite opacity that currently leaves
  midground buildings at roughly 69% effective opacity and near scenery at
  roughly 58%. Solid terrain, vegetation, buildings, bridges, and props must
  render with opaque interiors in day, sunset, and night; reserve alpha for
  transparent sprite backgrounds and intentionally soft atmospheric effects.
  Rebuild depth separation with layer-specific color grading, contrast,
  detail, scale, and dedicated ultra-far/far haze veils that sit between layers
  rather than washing through every object. Eliminate generic filters that
  unintentionally replace more specific depth treatment, and expose a small
  set of named CSS depth tokens so time palettes and parallax depth compose in
  one predictable place. Preserve route geometry, z-order, pointer inertness,
  palette crossfades, bounded DOM, train readability, and window transparency.
  Add focused style/computed-style tests proving opaque effective solids,
  monotonic depth contrast, atmosphere ownership, and unchanged geometry.
  Inspect the same seeded town and industrial chunks in day, sunset, and night
  with `borz` at compact and wide sizes, then run the frontend suite, build,
  and `make test`.

- [x] **TRAIN-028 — Rebuild the town-building lighting pipeline from daylight-neutral art.**
  Follow the imagegen skill in edit mode to revise the rowhouse, apartments,
  and cottage assets into crisp daylight-neutral pixel art with fully opaque
  wall/roof interiors, transparent exteriors, distinct materials, and no
  permanently lit windows. Preserve each asset's dimensions, anchor,
  silhouette, perspective, safe scale, and route placement. Create an exact
  geometry-aligned emissive mask or matched overlay for each asset so only its
  real windows can illuminate at sunset/night; remove the detached generic
  window bars for these buildings. The base art must remain readable after
  night grading without becoming translucent, while daylight must look
  inhabited but naturally unlit. Extend the manifest/rendering model without
  creating unbounded nodes or changing route output. Add deterministic alpha,
  dimension, alignment, palette-state, fallback/load, and bounded-DOM tests.
  Compare all three buildings in every time palette with `borz` at compact and
  wide sizes, then run the frontend suite, build, and `make test`.

- [x] **TRAIN-029 — Rebuild the industrial-building lighting pipeline from daylight-neutral art.**
  Apply the proven TRAIN-028 base-plus-emissive treatment to the workshop,
  warehouse, and water tower. Use the imagegen skill in edit mode; retain exact
  dimensions, anchors, silhouettes, perspective, and safe scale while replacing
  the current night-biased fill with opaque daylight materials and separating
  lamps/windows from the base pixels. Industrial structures should remain
  recognizably different from town housing in silhouette and material, with
  sparse plausible night lighting rather than every opening glowing. Remove any
  remaining detached generic building-window overlay and ensure missing
  emissive masks fail safely to an unlit solid building. Add asset-contract,
  alignment, palette isolation, regional ownership, deterministic rendering,
  and bounded-DOM tests. Compare industrial day, sunset, and night chunks with
  `borz` at compact and wide sizes, then run the frontend suite, build, and
  `make test`.

- [x] **TRAIN-030 — Regrade the distant terrain, coast, and bridge kit for clear daylight.**
  Audit the three terrain silhouettes, coast shore, and bridge truss against
  the new compositor. Use the imagegen skill in edit mode for any asset that
  cannot meet the visual contract through palette tokens alone. Daylight must
  show opaque, crisp, region-appropriate landforms and water edges with
  distinguishable depth planes; sunset/night may reduce chroma and value but
  must not reveal the sky through solid rock, shore, or bridge members.
  Preserve transparent exterior pixels, seamless chunk joins, anchors,
  perspective, collision widths, set-piece continuity, and the established
  shallow track perspective. Add representative alpha and color-range tests,
  seam and palette tests, plus deterministic bridge/coast traversal coverage.
  Inspect mountain, coast, and bridge scenes at compact and ultrawide sizes in
  all three palettes with `borz`, then run the frontend suite, build, and
  `make test`.

- [x] **TRAIN-031 — Regrade vegetation and expand the near-track prop vocabulary.**
  Audit the six vegetation sprites and three existing near-track props under
  the opaque compositor, editing night-biased or hazy assets with the imagegen
  skill while preserving their dimensions and anchors. Add at least five
  coherent near-track variants such as a milepost, signal cabinet, crossing
  marker, lamp post, and maintenance equipment, with explicit region pools,
  safe scales, collision widths, and day/night treatment. Solid foliage,
  trunks, fences, poles, and equipment must remain opaque and crisp; depth
  comes from palette/detail rather than transparency. Update cooldown and
  spacing rules so new props add rhythm without forming a picket fence,
  covering passengers, or repeating on adjacent chunks. Add asset alpha,
  manifest, regional-pool, spacing, cooldown, deterministic multi-seed, and
  bounded-DOM tests. Inspect every region in day and night at compact and wide
  sizes with `borz`, then run the frontend suite, build, and `make test`.

- [ ] **TRAIN-032 — Redesign daytime regional compositions and landmarks.**
  Rebalance forest, mountain, town, coast, and industrial profiles around the
  revised asset kit so each region has a readable daytime identity rather than
  the same dark silhouettes in different combinations. Add one restrained,
  deterministic daylight-readable landmark vocabulary per region—such as a
  forest clearing, mountain lookout or cabin, town church/market edge, coastal
  lighthouse/harbor cue, and industrial tanks/gantry—using the imagegen skill
  for raster additions. Keep landmarks sparse, reserve their spans before
  filler scenery, and give the journey alternating dense compositions and open
  views. Do not obscure the train, break transition grammar, exceed one major
  landmark per region, or increase the long-route DOM bound. Add per-region
  identity, landmark rarity, negative-space, incompatibility, determinism, and
  multi-seed distribution tests. Traverse all five regions in day mode with
  `borz` at compact and ultrawide sizes, then run the frontend suite, build,
  and `make test`.

- [ ] **TRAIN-033 — Build a richer deterministic day-and-sunset sky system.**
  Preserve TRAIN-023's seeded cloud grammar but retune cloud art and grading so
  daytime clouds are crisp white/blue-gray forms instead of translucent dark
  smudges, with sunset receiving deliberate warm rim/shadow treatment rather
  than a blanket sepia filter. Add restrained seeded sky anchors such as a
  high sun disk/halo, high wisps, and occasional weather variation while
  retaining believable open sky. Sky effects may be soft or translucent, but
  must remain behind terrain and never reduce train/control contrast. The same
  seed, viewport, and palette must reproduce the same catalogue; switching
  palette must not move clouds or regenerate geometry. Add determinism,
  distribution, open-space, palette-isolation, contrast, reduced-motion, and
  bounded-count tests. Inspect several seeds in day and sunset at compact,
  desktop, and ultrawide sizes with `borz`, then run the frontend suite, build,
  and `make test`.

- [ ] **TRAIN-034 — Diversify the deterministic night sky without artificial patterns.**
  Extend TRAIN-025 with a seeded celestial composition catalogue: varied moon
  phase, altitude, and horizontal position; adaptive moon-star exclusion; a
  very subtle airglow or Milky-Way-like band in only some seeds; and rare,
  restrained static celestial accents. Eliminate the single fixed full moon
  and ensure stars avoid rows, diagonals, uniform density, repeated clusters,
  terrain overlap, and identical refresh impressions when the journey seed
  differs. Day must hide all night-only elements; sunset may reveal only faint,
  plausible early elements. Keep every catalogue bounded, deterministic, and
  static under reduced motion. Add multi-seed phase/position variety,
  exclusion, lattice rejection, palette visibility, contrast, resize, cleanup,
  and DOM-bound tests. Inspect several seeds at compact and ultrawide night
  viewports with `borz`, then run the frontend suite, build, and `make test`.

- [ ] **TRAIN-035 — Add region-specific nighttime life and illumination.**
  Give every region a sparse nighttime signature using geometry-aligned light
  and region-owned details instead of uniformly darkening the daytime scene:
  examples include forest fireflies or a distant cabin, a mountain
  observatory/camp glow, town settlement glow and varied occupied windows,
  lighthouse/harbor lights with paired water reflections, and industrial
  beacons or restrained steam. Use the imagegen skill for raster landmarks and
  code-native overlays for light, plume, or reflection effects. Couple every
  emissive effect to a real owning asset/landmark, keep strong glow away from
  passengers, and prevent dense grids or every-building illumination. Day must
  hide night-only light while retaining any physical landmark. Add ownership,
  rarity, regional identity, reflection alignment, palette isolation,
  reduced-motion, deterministic multi-seed, and bounded-DOM tests. Traverse all
  five regions at night with `borz` at compact and wide sizes, then run the
  frontend suite, build, and `make test`.

- [ ] **TRAIN-036 — Add deterministic visual variants to major set pieces.**
  Create at least two coherent visual compositions for bridge, tunnel/cutting,
  coast reveal, and town edge while preserving their existing logical spans
  and station incompatibility rules. Reuse the revised kit where possible and
  use the imagegen skill for raster pieces that need a genuinely different
  silhouette. Select variants from the versioned route seed and set-piece ID;
  entry/body/exit must agree on one variant, connect continuously across chunk
  boundaries, and remain unchanged through palette switches and resize.
  Variants may change scenery composition but must not change station timing,
  route speed, track ownership, or train interaction geometry. Add selection,
  continuity, incompatibility, determinism, palette, long-route frequency, and
  bounded-DOM tests. Traverse every variant in day and night with `borz` at
  compact and ultrawide sizes, then run the frontend suite, build, and
  `make test`.

- [ ] **TRAIN-037 — Resume the deterministic journey across ordinary refreshes.**
  Persist a versioned journey snapshot in local storage containing the route
  seed and a station-safe route position/checkpoint so a normal refresh resumes
  the last scenery instead of always returning to the same opening chunk.
  Persist at a restrained cadence and on page lifecycle boundaries, never
  advance distance while the page is closed, and restore without a visible
  jump or station-state corruption. Validate schema/version/ranges, tolerate
  unavailable or malformed storage, and fall back deterministically; do not
  persist manual time-of-day overrides, live animation timestamps, private pane
  data, or unbounded history. Define a clear reset/migration path for future
  seed versions. Add reload/remount, cadence, malformed/stale data, storage
  failure, station approach/dwell/depart, reduced-motion, and geometry
  continuity tests. Use `borz` to prove a refresh resumes a nonzero day and
  night scene on port 5234, then run the frontend suite, build, and `make test`.

- [ ] **TRAIN-038 — Balance and harden the redesigned scenery system.**
  Perform a final multi-seed, long-route pass across all five regions, all
  palettes, every set piece, station states, refresh restore, compact desktop,
  normal desktop, and ultrawide layouts. Tune density, cooldowns, contrast,
  haze depth, landmark rarity, night light strength, open-sky/open-land gaps,
  and transition pacing so no palette is empty, muddy, overfilled, repetitive,
  or visually detached from the train. Add statistical regression bounds for
  regional variety and rare-feature cadence, assert solid-sprite effective
  opacity and emissive ownership, and retain bounded DOM/memory and acceptable
  animation behavior. Fix only defects attributable to the redesigned scenery;
  do not expand into train artwork or unrelated UI. Run the complete frontend
  suite, build, `make test`, and `rtk git diff --check`, then perform and record
  a sustained `borz` smoke traversal following TRAIN-015's exact viewport and
  module-freshness procedure.

