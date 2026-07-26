# Train Layout — Visual Recovery Work Items

Completed work is archived in:

- [TRAIN-001–026](docs/archive/train-layout-workitems-001-026.md)
- [TRAIN-027–042](docs/archive/train-layout-workitems-027-042.md)

This active queue follows a train-free, multi-position audit of the current
scene at commit `190ec6b`. The audit found systemic composition defects rather
than one remaining transparency bug: the station is now an opaque monolithic
wall; terrain reads as stacked horizontal bands; repeated mountains and mesas
flatten regional identity; scenery scale and ground contact are inconsistent;
and named bridge, tunnel, town-edge, and coast-reveal captures often did not
contain the named set piece. The first item therefore repairs event
choreography and visual proof before later items revise artwork.

## Worker contract

- Work from this repository's root and read `AGENTS.md`, `CLAUDE.md`, and this
  file completely before starting an item.
- Select only the first unchecked item. Never start or partially implement a
  second item in the same cycle.
- The existing train, locomotive, carriages, track, seats, characters,
  transparent windows, fixed-train/world-motion separation, route timing,
  station state, and click targets are the baseline. Do not redraw or
  restructure them unless the selected item explicitly requires an integration
  change.
- A clean worktree is the normal starting condition. Follow the dedicated
  worker recovery protocol in the loop prompt for an interrupted item. Never
  reset, clean, stash, overwrite, or discard pre-existing work.
- Keep the train visually fixed while scenery moves from left to right because
  the locomotive faces left. Rails remain world-owned and must not move with
  the train.
- Keep route generation deterministic and bounded. The same seed, journey
  position, viewport, and time input must reproduce the same scene. Do not use
  unbounded DOM, uncontrolled per-frame randomness, or one enormous backdrop.
- Solid terrain, vegetation, architecture, props, and physical fixtures must
  have opaque interiors. Reserve alpha for transparent exteriors and
  intentionally soft atmosphere, steam, glow, and reflections. Build depth
  through scale, value, chroma, contrast, detail, overlap, and separate haze
  planes—not translucent solid objects.
- Keep illumination geometry attached to the object that owns it. Daylight
  must not contain permanently emissive windows or lamps.
- Preserve crisp coherent pixel art, nearest-neighbour rendering, explicit
  ground anchors, restrained detail by depth, and clean silhouettes. Do not
  introduce photographic blur, antialiased halos, matte fringes, floating
  sprites, or accidental semi-transparent fill.
- Respect `prefers-reduced-motion`, tab visibility, viewport resizing, and the
  existing theme/settings architecture. Add focused deterministic Vitest
  coverage for every behavioral or geometry contract.
- Use the imagegen skill only when the selected item explicitly requires
  raster asset generation or editing. Inspect every generated asset against
  its alpha, anchor, silhouette, scale, and pixel-art acceptance criteria.
- Use `borz` and only the shared Vite server at `http://127.0.0.1:5234/`.
  Follow the loop prompt's exact HTTP 200, plain-module freshness,
  open/viewport/hard-reload sequence before trusting browser output. Never
  launch an alternate port.
- Every visual acceptance pass must temporarily hide
  `.train-layout-inspection`, `.train-world-debug-grid`, and
  `.train-time-toggle` in the live DOM so the complete scenery is visible.
  Restore the DOM afterward. Capture at least compact and desktop views in day,
  sunset, and night; use ultrawide where the item changes long compositions or
  repetition. A route label, DOM node, metadata value, test name, or screenshot
  filename is not visual proof: inspect the retained image and assert that the
  intended object has a meaningful visible bounding box.
- Run targeted tests while implementing. Before completion, run the frontend
  suite and build; run `make test` when shared application behavior or
  Go-embedded web output is affected. Run `rtk git diff --check`.
- Complete one item atomically: implementation, tests, required assets or
  documentation, retained visual evidence notes, and exactly that item's
  checkbox belong in one commit. Do not push.
- If blocked, keep the item unchecked and do not commit partial work. Preserve
  and report every dirty path and exact error. If all items are checked, make
  no changes and report `QUEUE_COMPLETE`.

## Audit defects to eliminate

The deterministic train-free matrix covered route positions `0`, `2880`,
`6720`, `11520`, `28800`, `122240`, and `123840` in day, sunset, and night.
Treat these findings as regression targets:

- Station segments merge into a broad, nearly featureless rectangular wall.
- Ultra-far, far, midground, and near terrain form large horizontal colour
  slabs; contour variation is too weak to read as land.
- Similar-size mountain and mesa silhouettes repeat in unrelated regions,
  producing wallpaper rather than atmospheric depth.
- Tiny buildings and props appear pasted onto the track line or float against
  the terrain because scale and ground ownership are inconsistent.
- Forest, mountain, town, coast, and industrial regions lack distinct
  silhouettes, materials, density rhythms, and negative space.
- Day, sunset, and night mostly recolour the same composition; sunset is a
  blanket tint and night lacks local, region-owned life.
- Set pieces are assigned logical route chunks but may render on `far` or
  `midground` tracks moving at `0.25` or `0.55` speed. Existing route-position
  captures used near-layer coordinates, so the named object could be offscreen
  while metadata still claimed success.

## Queue

- [x] **TRAIN-043 — Synchronize set-piece choreography and make visual proof trustworthy.**
  Establish one logical journey anchor for every bridge, tunnel, town-edge,
  coast-reveal, and station composition. Project its participating geometry
  into the owning parallax layers so entry, body, exit, reserved clearings, and
  supporting terrain meet in the viewport at the same journey moment even when
  those layers use different speed ratios. A midground or far set piece must
  not appear later inside an unrelated near-layer region, and reservations
  must suppress collisions at the actual rendered screen position. Preserve
  ordinary parallax speed, deterministic region generation, station timing,
  seam overlap, reduced motion, and bounded windows/DOM.

  Add a deterministic focus/diagnostic helper that locates a requested
  set-piece occurrence and returns the journey position needed to centre its
  *actual render-layer geometry*, plus its expected visible segment IDs. In
  tests and browser acceptance, require the named composition's centre to fall
  within the central half of the viewport and its union to visibly intersect
  at least `min(320px, 50% of viewport width)`; reject metadata-only or
  edge-sliver passes. Add cross-layer screen-coordinate tests at several
  positions and speed ratios, collision-reservation tests, focus round trips,
  and bounded-window regressions. With the train hidden, retain compact,
  desktop, and ultrawide day screenshots of all five set-piece types and
  inspect the images before marking the item complete.

- [x] **TRAIN-044 — Replace the monolithic station wall with a readable station campus.**
  Recompose the six near-layer station segments into a continuous but
  articulated station: deliberate entry and exit platforms, a smaller station
  house, canopy bays and supports, doors and windows, baggage/service elements,
  and framed open-air gaps that reveal plausible scenery. No solid façade,
  roof, platform, or canopy may span most of the viewport as a featureless
  rectangle. Architectural ends must be visible at compact size, while desktop
  and ultrawide views must read as one campus rather than copied tiles.
  Physical surfaces remain opaque; lamps, windows, and signals retain their
  day-unlit/sunset-restrained/night-owned illumination.

  Preserve station arrival/departure timing and state, train/track geometry,
  route choreography from TRAIN-043, clean chunk joins, pointer inertness, and
  DOM bounds. Add focused segment-role, join, opening-ownership, fixture-count,
  opacity, and time-lighting tests. With the train hidden, capture the complete
  first station at entry, centre, and exit in all three palettes at compact,
  desktop, and ultrawide sizes; inspect both the full composition and isolated
  near layer.

- [x] **TRAIN-045 — Rebuild terrain silhouettes and remove horizontal colour slabs.**
  Redesign ultra-far, far, midground, and near terrain bases so each layer has
  a legible irregular silhouette, intentional overlaps, and region-owned
  materials instead of stacked full-width bands. Increase meaningful contour
  amplitude without exposing sky holes, track gaps, or chunk seams. Provide
  coherent transitions between forest soil, mountain rock, town ground, coast
  shore/water, and industrial fill. The near track bed remains stable and the
  train continues to cover the rail contact area naturally.

  Add deterministic contour-envelope, seam-continuity, minimum-variation,
  material-ownership, and no-gap tests across multiple seeds and region
  boundaries. With the train hidden, isolate every depth layer and retain
  compact/desktop day screenshots for all five regions plus desktop
  sunset/night composites. Reject any result that still reads as four
  near-horizontal stripes.

- [x] **TRAIN-046 — Enforce depth, scale, overlap, and ground-contact grammar.**
  Define and apply a measurable visual grammar: ultra-far and far assets are
  smaller, lower-contrast, less detailed, and atmospherically separated;
  midground assets establish regional silhouettes; near assets are darker,
  larger, sparse enough to preserve train readability, and anchored to their
  owning terrain contour. Eliminate same-size mountains across layers, props
  sitting on the rail line, floating buildings, buried bases, and detached
  emissive overlays. Keep haze on dedicated planes rather than inside opaque
  sprites.

  Add manifest/placement tests for depth scale ranges, contrast ordering,
  anchor-to-contour tolerance, overlap bounds, collision spacing, and
  illumination ownership. Audit representative ordinary chunks from all five
  regions with the train hidden in day and night at compact, desktop, and
  ultrawide widths; retain isolated-layer images proving monotonic depth.

- [x] **TRAIN-047 — Recompose ordinary forest and mountain scenery.**
  Give forest and mountain routes distinct readable identities using the
  corrected terrain/depth grammar. Forest needs varied tree families, canopy
  clusters, undergrowth, clearings, streams or fences, and occasional small
  human-scale landmarks. Mountain needs layered ridges, cliffs, rock fields,
  alpine vegetation, cabins or lookouts, and deliberate open vistas. Remove
  the repeated mountain/mesa wallpaper, mirrored clusters, uniform density,
  and asset combinations that ignore region material or scale.

  Use imagegen in edit/generation mode only for raster sprites that cannot be
  composed cleanly from current assets; keep deterministic variants and
  anchors. Add regional-pool, density/gap rhythm, repetition-distance,
  silhouette-diversity, landmark-frequency, and transition tests across
  multiple seeds. Retain train-free compact/desktop/ultrawide day and night
  sequences covering ordinary forest, forest→mountain, mountain, and
  mountain→forest travel.

- [x] **TRAIN-048 — Recompose ordinary town and industrial scenery.**
  Make town read as coherent settlement blocks with streets, yards, fences,
  trees, civic/commercial accents, and varied but compatible building scales.
  Make industrial routes read through sheds, tanks, stacks, cranes, utility
  lines, service roads, and controlled negative space. Replace tiny pasted
  buildings, arbitrary isolated props, repeated identical façades, and objects
  that sit on the track line. Keep every solid surface opaque and attach
  window, lamp, signal, and furnace light to its owning geometry.

  Use imagegen where raster asset revisions are required. Add block-composition,
  scale-family, ground-anchor, spacing, repetition, solid-alpha, and
  day/sunset/night emissive tests across several seeds. Retain train-free
  compact/desktop/ultrawide sequences for town, town→industrial, industrial,
  and industrial→town in all three palettes.

- [ ] **TRAIN-049 — Recompose coast, shore, and water depth.**
  Make coast immediately readable without relying on metadata: establish an
  actual shore profile, water planes with depth-appropriate movement cues,
  beaches or rock shelves, harbour details, sparse vegetation, and occasional
  lighthouse, pier, boat, or navigation accents. Remove white horizontal bars,
  mountain-dominated coast views, detached water fragments, and shorelines
  hidden behind unrelated terrain. Reflections may be translucent but must be
  clipped to water and owned by the reflected source.

  Use imagegen only for required raster scenery. Add shore-continuity,
  water-ownership, reflection-clipping, regional-pool, landmark-spacing, and
  forest/town/industrial transition tests. Retain train-free compact, desktop,
  and ultrawide day/sunset/night sequences that show a coast arrival, ordinary
  coast travel, and coast departure with visibly continuous water.

- [ ] **TRAIN-050 — Rebuild bridge and tunnel as unmistakable traversals.**
  Recompose both bridge variants so approach, supported span, structure below
  the track, and exit form one visible crossing rather than a detached truss.
  Recompose both tunnel variants so approach cutting, portal, dark opening,
  enclosing mountain mass, and exit read as a passage rather than repeated
  rocks or white strips. Coordinate all participating depth layers through
  TRAIN-043 and reserve only the geometry each traversal actually needs.
  Preserve train/rail alignment, direction of travel, event spacing, and
  regional transitions.

  Use imagegen if raster structures or terrain masks require replacement. Add
  variant/role continuity, portal/span geometry, track-contact,
  cross-layer-alignment, reservation, and visible-focus tests. With the train
  hidden, retain entry/centre/exit compact, desktop, and ultrawide sequences
  for both variants in day and night. Each image must visibly contain the named
  structure, not merely its route metadata.

- [ ] **TRAIN-051 — Rebuild town-edge and coast-reveal as readable transitions.**
  Recompose both town-edge variants into a deliberate change from open land to
  coherent settlement density, with a visible edge, road/yard grammar, and
  compatible foreground clearings. Recompose both coast-reveal variants so
  terrain opens to a broad, unmistakable first water view with shoreline
  framing and depth, not a small shore sprite hidden behind mountains.
  Coordinate entry/body/exit geometry across layers, preserve deterministic
  region order and spacing, and prevent either transition from colliding with
  stations or the bridge/tunnel traversals.

  Use imagegen where required. Add variant distinction, transition-gradient,
  cross-layer screen alignment, collision exclusion, visible-water/settlement
  coverage, and focus-helper tests. Retain train-free entry/centre/exit
  compact, desktop, and ultrawide sequences for both variants in day and night;
  inspect that the intended transition is centred and immediately legible.

- [ ] **TRAIN-052 — Rebalance sky, atmosphere, and regional time-of-day life.**
  After geometry and regional art are stable, replace blanket recolouring with
  palette-owned lighting. Day needs a convincingly blue sky, clear depth, and
  naturally unlit fixtures. Sunset needs localized warm horizon light and
  cooler depth rather than a uniform purple/brown wash. Night needs believable
  blue-black depth, sparser natural star groupings, varied celestial placement,
  and region-owned pools of attached windows, lamps, signals, fires, and water
  reflections. Vary cloud groups, gaps, altitude, scale, and drift by seed
  without letting sky objects move at train/near-layer speed.

  Add palette-token, luminance-order, emissive-ownership, star-distribution,
  cloud-spacing/speed, and deterministic multi-seed tests. Retain train-free
  compact/desktop/ultrawide day, sunset, and night matrices for all five
  regions and every major set piece. Confirm all solid objects remain opaque
  and regional geometry remains readable in every palette.

- [ ] **TRAIN-053 — Run multi-seed visual convergence and lock final regressions.**
  Perform a final train-free audit of at least five deterministic seeds across
  compact, desktop, and ultrawide viewports, covering ordinary chunks,
  boundaries, every station phase, and both variants of bridge, tunnel,
  town-edge, and coast-reveal in day, sunset, and night. Inspect retained
  contact sheets for repeated wallpaper, horizontal slabs, monolithic blocks,
  floating or pasted assets, opaque-window regressions, unreadable regions,
  collisions, seams, gaps, clipped stations, and offscreen named set pieces.
  Fix only defects within this final integration scope; do not redesign the
  train or introduce a new scenery system.

  Add statistical regressions for bounded DOM, route determinism, repetition
  distance, regional coverage, set-piece visibility, depth ordering,
  solid-alpha ownership, palette contrast, and reduced-motion behavior. Run
  the complete frontend suite, production build, and `make test`. Retain a
  labelled final contact sheet and document the seeds, route positions,
  viewport sizes, palettes, and visible set-piece bounds used for acceptance.
