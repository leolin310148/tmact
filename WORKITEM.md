# Train Layout — Red-Team Visual Repair Work Items

Completed work is archived in:

- [TRAIN-001–026](docs/archive/train-layout-workitems-001-026.md)
- [TRAIN-027–042](docs/archive/train-layout-workitems-027-042.md)
- [TRAIN-043–053](docs/archive/train-layout-workitems-043-053.md)

This queue follows an independent visual re-review of commit `a03543e`. The
tests and the previous 117-capture report were green, but direct inspection of
the committed
[TRAIN-053 contact sheet](docs/train-053-final-contact-sheet.png) and fresh
live captures proved that the art still fails: terrain is covered by dominant
crosshatched wallpaper, ordinary forest has almost no forest, town assets mix
incompatible pixel densities and scales, coast buildings appear to stand in
water, the bridge reads as a viewport-wide truss screen, and the tunnel is a
giant black rectangle disconnected from the rails. Station clipping improved,
but the campus is still overly wall-like.

The source causes are now known. Terrain material selectors use multiple
full-surface diagonal `repeating-linear-gradient` layers. Traversal and
transition compositions render in every reserved parallax layer, so the
current DOM contains two visible bridge/tunnel copies and three coast-reveal
copies. Tunnel body CSS expands its opening to both chunk edges and removes its
arch. These are repair targets, not acceptable stylistic variants.

## Worker contract

- Work from this repository's root and read `AGENTS.md`, `CLAUDE.md`, and
  this file completely before starting an item.
- Select only the first unchecked item. Never start or partially implement a
  second item in the same cycle.
- A clean worktree is the normal starting condition. Follow the dedicated
  worker recovery protocol in the loop prompt for interrupted work. Never
  reset, clean, stash, overwrite, or discard pre-existing changes.
- Preserve the existing fixed train, transparent windows, rotating wheels,
  rails, characters, enlarged click targets, route/station timing, seeded
  determinism, fixed-train/world-motion separation, and train/office shared
  layout height unless the selected item explicitly requires an integration
  adjustment.
- Keep solid scenery opaque. Use alpha only for transparent exteriors and
  intentionally soft clouds, atmosphere, steam, glow, water highlights, or
  reflections. Depth comes from scale, value, chroma, overlap, detail, and
  dedicated haze—not translucent buildings or terrain.
- Preserve coherent pixel art: compatible pixel density, perspective, palette,
  ground anchors, nearest-neighbour edges, and bounded DOM. Use the imagegen
  skill only when the item explicitly requires raster generation or editing.
- Use `borz` with the dedicated `tmact-train-workitems` profile and only the
  shared Vite server at `http://127.0.0.1:5234/`. Follow the loop prompt's
  exact HTTP 200, plain-module freshness, open, viewport, hard-reload, and
  viewport-assertion procedure. Never use another port.
- Every visual item must be checked in the real `184px` train-layout band.
  Obtain `.train-layout.getBoundingClientRect()` after selecting a pane and
  crop the screenshot to that exact band; a terminal-sized black area is not
  evidence. Temporarily hide `.train-layout-inspection`,
  `.train-world-debug-grid`, and `.train-time-toggle` only for scenery-only
  captures, then restore the DOM.
- For traversal geometry, also retain train-visible entry/body/exit captures.
  The rails, wheels, deck, portal, platform, and train contact must be judged
  together. Do not resize the layout, move it to a proof-only geometry, or
  hide a defect behind the train.
- A route label, focus parameter, DOM metadata, bounding-box assertion,
  screenshot filename, test name, or green suite is not visual acceptance.
  Open and inspect every retained image. If a listed defect is still visibly
  present, keep the item unchecked even when all automated tests pass.
- Run targeted Vitest during implementation. Before completion run the full
  frontend suite and production build; run `make test` when shared behavior or
  embedded web output is affected. Run `rtk git diff --check`.
- Complete exactly one item atomically: implementation, tests, evidence notes
  or assets, and only that item's checkbox belong in one commit. Do not push.
- If blocked, do not check or commit partial work. Preserve and report the
  exact dirty paths and error. If every item is checked, make no changes and
  report `QUEUE_COMPLETE`.

## Reproduction baseline

Use the listed desktop checks first, then the per-item compact/night variants:

- Ordinary forest: seed `train-053-aurora`, route position `7199`.
- Ordinary town: seed `train-053-aurora`, route position `1439`.
- Ordinary coast: seed `train-053-aurora`, route position `128479`.
- Bridge: seed `train-053-aurora`, focus `bridge`, occurrence `0`.
- Tunnel: seed `train-053-cascade`, focus `tunnel`, occurrence `0`.
- Station: seed `train-053-summit`, focus `station`, occurrence `0`.
- Coast reveal: seed `train-053-orchard`, focus `coast-reveal`, occurrence `0`.

Set `train-cruise-speed=0.001` while composing still captures. Validate motion
separately at the default speed. The fresh red-team review used `1280×800` and
found the real layout band at approximately `y=573`, height `184`; always use
the measured current rectangle rather than hard-coding that coordinate.

## Queue

- [x] **TRAIN-054 — Remove terrain crosshatch wallpaper and restore restrained materials.**
  Remove the full-chunk diagonal and orthogonal
  `repeating-linear-gradient` stacks from the five
  `.train-terrain-base[data-terrain-material]` selectors. Replace them with
  sparse, region-owned pixel details or contour accents that occupy a
  minority of each surface and never form a viewport-wide lattice. Forest
  soil may use small grass/leaf clusters, mountain rock short strata and
  isolated facets, town ground restrained block/road seams, coast shore
  broken rock/sand marks, and industrial fill sparse engineered seams.
  Preserve opaque fills, irregular contour silhouettes, chunk continuity,
  distinct depth values, and the non-terrain patterns used by rails and
  architecture.

  Add focused stylesheet/DOM tests rejecting diagonal repeating gradients in
  terrain material rules, limiting material accents to owned pseudo-elements
  or bounded details, and preserving material identity, opacity, seams, and
  palette ordering. Reproduce ordinary forest `7199`, town `1439`, coast
  `128479`, and a mountain sample from the previous contact sheet in day and
  night at desktop, plus forest/town at compact. Inspect cropped train-free
  images and explicitly reject any remaining fence, scaffold, diamond-grid,
  or full-surface stripe reading before checking the item.

- [x] **TRAIN-055 — Restore ordinary forest and mountain compositions after material cleanup.**
  Recompose ordinary forest so it visibly contains layered tree and shrub
  families, clearings, ground vegetation, and occasional human-scale details;
  the forest sample must not be only mountains and terrain. Recompose mountain
  scenes around ridges, rock faces, alpine vegetation, and negative-space
  vistas rather than repeated orange mesas or the same snowy peak at multiple
  equal scales. Keep far silhouettes subordinate and reserve the near band for
  sparse anchored elements that do not obscure passengers.

  Reuse or edit current raster assets with imagegen when required. Add regional
  pool, minimum visible vegetation, scale/depth family, repetition-distance,
  anchor, and transition tests across at least five seeds. Retain cropped
  train-free forest and mountain day/sunset/night captures at compact,
  desktop, and ultrawide widths. Compare them directly with the corresponding
  TRAIN-053 contact-sheet tiles; forest must read as forest at a glance, and
  neither region may recover the removed crosshatch as a new pattern.

- [x] **TRAIN-056 — Normalize town and industrial pixel scale, perspective, and grounding.**
  Remove the collage effect in the town `1439` sample: do not mix crude
  flat CSS boxes beside highly detailed raster churches and rowhouses at
  incompatible pixel density or scale. Establish a small coherent building
  scale family and perspective, align foundations to owned town ground, use
  streets/yards/fences to connect structures, and keep deliberate gaps.
  Apply the same grammar to industrial sheds, stacks, tanks, gantries, and
  service areas. Buildings may vary in size, but doors, windows, storeys, and
  ground contact must remain mutually believable.

  Use imagegen for raster revisions when needed rather than approximating a
  detailed sprite with an unrelated CSS rectangle. Add tests for scale-family
  bounds, pixel-density metadata, foundation-to-contour tolerance, fixture
  ownership, collision spacing, and daylight-unlit emissives. Retain cropped
  train-free ordinary town and industrial comparisons in all three palettes
  at compact, desktop, and ultrawide widths; reject floating bases, mixed art
  styles, or a line of disconnected dollhouses.

- [x] **TRAIN-057 — Rebuild coast grounding and make coast reveal genuinely distinct.**
  Keep continuous readable water, but establish dry shoreline shelves,
  embankments, piers, or harbour ground under every land-owned building and
  fixture. Only boats, buoys, reflections, and water cues may occupy water.
  Ordinary coast must not look like town buildings placed directly on a blue
  rectangle. Recompose coast-reveal as a transition from enclosed land to a
  broad first water view with shoreline framing and fewer structures; it must
  be visually distinct from ordinary coast.

  Split coast-reveal participation by layer: far owns water/horizon,
  midground owns shore/framing, and near owns only track-adjacent foreground.
  Do not render three complete copies of the same composition. Add layer
  ownership, single-owner geometry, dry-land foundation, water clipping,
  reflection, transition, collision, and focus visibility tests. Reproduce
  ordinary coast `128479` and orchard coast-reveal occurrence `0` with
  train-hidden and train-visible day/night captures at compact, desktop, and
  ultrawide widths. Reject any building whose base visually terminates inside
  water.

- [x] **TRAIN-058 — Rebuild tunnel traversal around a real rail-aligned portal and bore.**
  Replace the current three-piece giant black rectangle. Entry and exit must
  each present one coherent portal embedded in rock; the body may darken the
  scene as the train passes inside, but must retain rock lining, rail/deck
  continuity, controlled occlusion, and train readability. Never expand a body
  opening edge-to-edge with `border-radius: 0`, and do not show simultaneous
  unrelated side holes in the centred composition. The fixed train's wheel/
  rail line must enter the portal instead of passing below it.

  Give primary and supporting layers distinct responsibilities rather than
  drawing two complete tunnel copies. Add entry/body/exit state geometry,
  single-opening, portal silhouette, rail-contact, layer-ownership, collision,
  reduced-motion, and both-variant tests. For each variant, retain cropped
  train-hidden and train-visible entry/body/exit sequences in day and night at
  compact and desktop, plus one ultrawide sequence. Reject any broad flat black
  rectangle, detached portal, duplicated bore, or rail line outside the
  opening.

- [x] **TRAIN-059 — Rebuild bridge traversal as a crossing rather than a truss screen.**
  Establish a clear crossing subject beneath the rails—river, gorge, or
  lowered terrain—with approaches, deck, supports, and restrained truss or
  railing. Truss members must frame the train and remain local to the bridge;
  they must not cover most mountains or resemble the terrain crosshatch.
  Rails and wheels must align with the bridge deck throughout entry, span, and
  exit. Both variants need visibly different but compatible structure.

  Split primary/supporting layer geometry so the DOM does not paint two full
  bridge compositions on top of each other. Add single-owner truss/deck,
  crossing-void, support-below-deck, rail-contact, entry/body/exit continuity,
  variant, collision, and focus tests. Retain cropped train-hidden and
  train-visible entry/span/exit sequences for both variants in day and night
  at compact and desktop, plus one ultrawide sequence. Reject a viewport-wide
  lattice, missing crossing void, duplicated truss, or train floating above/
  below the deck.

- [ ] **TRAIN-060 — Refine the station into a lighter articulated campus.**
  Preserve the repaired day/night joins and fixture lighting, but reduce the
  remaining continuous industrial-wall impression. Use shorter station-house
  masses, open platform stretches, canopy gaps, platform furniture, explicit
  entrances, and scenery-visible negative space. Keep solid architecture
  opaque and ensure the fixed train naturally overlaps the platform edge
  without concealing all station identity.

  Add campus-mass coverage, negative-space, segment-role, train/platform
  overlap, fixture sparsity, day/night illumination, and lifecycle regression
  tests. Reproduce summit station entry, centre, exit, approach, platform,
  dwell, and departure at compact/desktop/ultrawide in day and night. Inspect
  both train-hidden and train-visible crops; reject a broad featureless wall,
  clipped ends, floating canopy, or any return of the previous diagonal cut.

- [ ] **TRAIN-061 — Perform an adversarial visual audit and lock only observed fixes.**
  Run a fresh red-team audit rather than extending the previous pass report.
  Cover at least five seeds; ordinary forest, mountain, town, coast, and
  industrial scenes; all boundaries; station phases; and both variants of
  bridge, tunnel, town-edge, and coast-reveal. Use compact `390×844`, desktop
  `1280×800`, and ultrawide `2560×900`, with day, sunset, and night where
  applicable. Capture the measured real train-layout band, not the surrounding
  terminal. Include train-hidden scenery crops and train-visible contact crops
  for bridge, tunnel, station, and coast grounding.

  Create a labelled committed before/after contact sheet that places the
  relevant TRAIN-053 failure tile beside the new result. Explicitly inspect
  and document: crosshatch coverage, forest vegetation, repeated mountain
  scale, town pixel-density cohesion, land/water grounding, bridge crossing
  subject and deck contact, tunnel portal/bore/rail contact, station negative
  space, palette readability, seams, collisions, and solid opacity. Add
  regression tests for the concrete contracts introduced by TRAIN-054–060,
  but do not cite test success as visual proof. If any enumerated defect remains
  visible, fix it within this integration item or leave TRAIN-061 unchecked and
  report the exact screenshots. Run the full frontend suite, production build,
  `make test`, and `rtk git diff --check`.
