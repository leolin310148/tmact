// TrainLayout — train-theme pane switcher prototype.
//
// Panes use the same visible/overflow split as the chip and office switchers.
// Visible panes are packed four per double-decker carriage; overflow panes and
// recently-closed sessions live behind the locomotive. The carriage, empty
// chair, and occupied chair are separate aligned layers. Seat order is
// upper-left, upper-right, lower-left, lower-right; right-side chair sprites
// mirror the shared right-facing artwork so every pair faces inward.
// The scenery uses a separate, bounded route-chunk window behind the consist.

import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { createPortal } from "react-dom";
import { onPointerDownNoBlur } from "../lib/dom";
import type { PaneStatus } from "../types/server";
import carriageUrl from "../assets/train-theme/sprites/train-carriage-empty-v2.png";
import locomotiveUrl from "../assets/train-theme/sprites/train-locomotive.png";
import emptySeatUrl from "../assets/train-theme/sprites/characters/train-seat-empty-v2.png";
import occupiedSeat01Url from "../assets/train-theme/sprites/characters/train-seat-person-01-v2.png";
import occupiedSeat02Url from "../assets/train-theme/sprites/characters/train-seat-person-02-v2.png";
import occupiedSeat03Url from "../assets/train-theme/sprites/characters/train-seat-person-03-v2.png";
import occupiedSeat04Url from "../assets/train-theme/sprites/characters/train-seat-person-04-v2.png";
import occupiedSeat05Url from "../assets/train-theme/sprites/characters/train-seat-person-05-v2.png";
import occupiedSeat06Url from "../assets/train-theme/sprites/characters/train-seat-person-06-v2.png";
import {
  paneListItems,
  panePeer,
  paneStateClass,
  paneStateLabel,
  splitPaneItems,
  type PaneListItem,
} from "./StatusLine";
import { OverflowMenuContent, useMenuPopover } from "./OverflowMenu";
import {
  DEFAULT_TRAIN_ROUTE_SEED,
  RouteChunkWindow,
  routeChunkWindowRange,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_PARALLAX_SEAM_OVERLAP,
  TRAIN_ROUTE_CHUNK_WIDTH,
  TRAIN_ROUTE_SEED_VERSION,
  trainParallaxLayerPosition,
  type RouteChunk,
  type RouteChunkWindowSnapshot,
  type TrainParallaxLayer,
  type TrainParallaxLayerName,
} from "./trainRoute";
import {
  trainSceneryPlacementsForChunk,
} from "./trainScenery";
import "./TrainLayout.css";

interface TrainLayoutProps {
  panes: PaneStatus[];
  selected: string | null;
  onSelect: (paneID: string) => void;
}

const SEAT_CLASSES = [
  "upper-left",
  "upper-right",
  "lower-left",
  "lower-right",
] as const;

const OCCUPIED_SEAT_URLS = [
  occupiedSeat01Url,
  occupiedSeat02Url,
  occupiedSeat03Url,
  occupiedSeat04Url,
  occupiedSeat05Url,
  occupiedSeat06Url,
];

const TRAIN_WORLD_SPEED_PX_PER_SECOND = 12;
const TRAIN_WORLD_DEBUG_PARAM = "train-world-debug";
const TRAIN_WORLD_SEED_PARAM = "train-route-seed";

// Route position increases as the left-facing train travels forward. CSS uses
// that positive value as the route layer's x translation, so the world moves
// right while the consist itself remains fixed.
export function advanceTrainWorldRoutePosition(
  routePosition: number,
  elapsedMs: number,
  speedPxPerSecond = TRAIN_WORLD_SPEED_PX_PER_SECOND,
): number {
  return routePosition + (Math.max(0, elapsedMs) * speedPxPerSecond) / 1000;
}

export function trainWorldDebugEnabled(search: string): boolean {
  return (
    import.meta.env.DEV &&
    new URLSearchParams(search).get(TRAIN_WORLD_DEBUG_PARAM) === "1"
  );
}

export function trainWorldRouteSeed(search: string): string {
  if (!import.meta.env.DEV) return DEFAULT_TRAIN_ROUTE_SEED;
  const requested = new URLSearchParams(search).get(TRAIN_WORLD_SEED_PARAM)?.trim();
  return requested ? requested.slice(0, 64) : DEFAULT_TRAIN_ROUTE_SEED;
}

// Calculate the least number of carriage layers needed for the consist to
// reach the viewport's right edge. CSS uses negative margins to overlap the
// couplers, so measurements use each flex item's outer width rather than its
// raw image width.
export function minimumCarriagesForWidth(
  viewportWidth: number,
  locomotiveOuterWidth: number,
  carriageOuterWidth: number,
): number {
  if (viewportWidth <= 0 || carriageOuterWidth <= 0) return 1;
  const remaining = Math.max(0, viewportWidth - Math.max(0, locomotiveOuterWidth));
  return Math.max(1, Math.ceil(remaining / carriageOuterWidth));
}

function outerWidth(element: HTMLElement): number {
  const style = window.getComputedStyle(element);
  const marginLeft = Number.parseFloat(style.marginLeft) || 0;
  const marginRight = Number.parseFloat(style.marginRight) || 0;
  return element.getBoundingClientRect().width + marginLeft + marginRight;
}

function TrainPassenger({
  item,
  globalIndex,
  seatIndex,
  selected,
  onSelect,
}: {
  item: PaneListItem;
  globalIndex: number;
  seatIndex: number;
  selected: boolean;
  onSelect: (paneID: string) => void;
}) {
  const ref = useRef<HTMLButtonElement | null>(null);
  const { pane, label } = item;
  const paneID = pane.pane_id ?? "";
  const peer = panePeer(pane);
  const side = seatIndex % 2 === 0 ? "right" : "left";
  const seatClass = SEAT_CLASSES[seatIndex]!;
  const characterIndex = globalIndex % OCCUPIED_SEAT_URLS.length;

  useLayoutEffect(() => {
    if (selected && ref.current && typeof ref.current.scrollIntoView === "function") {
      ref.current.scrollIntoView({ block: "nearest", inline: "nearest" });
    }
  }, [selected]);

  const className = [
    "train-seat",
    `train-seat--${seatClass}`,
    `train-seat--facing-${side}`,
    `state-${paneStateClass(pane)}`,
    selected ? "selected" : "",
    pane.asking ? "asking" : "",
    pane.stale ? "stale" : "",
  ]
    .filter(Boolean)
    .join(" ");
  const displayName = (peer ? peer + " " : "") + label;

  return (
    <button
      ref={ref}
      type="button"
      className={className}
      title={`${displayName} — ${paneStateLabel(pane)}`}
      aria-label={`Select pane ${displayName}, ${paneStateLabel(pane)}`}
      aria-pressed={selected}
      data-pane-id={paneID}
      data-seat-index={seatIndex}
      data-character-index={characterIndex}
      onPointerDown={onPointerDownNoBlur}
      onClick={() => onSelect(paneID)}
    >
      <img
        className="train-seat-sprite train-seat-sprite--occupied"
        src={OCCUPIED_SEAT_URLS[characterIndex]}
        alt=""
        draggable={false}
      />
      {pane.asking && !pane.stale ? (
        <span className="train-person-ask" aria-hidden="true">
          ?
        </span>
      ) : null}
    </button>
  );
}

function EmptyTrainSeat({ seatIndex }: { seatIndex: number }) {
  const side = seatIndex % 2 === 0 ? "right" : "left";
  const seatClass = SEAT_CLASSES[seatIndex]!;

  return (
    <span
      className={[
        "train-seat",
        "train-seat--empty",
        `train-seat--${seatClass}`,
        `train-seat--facing-${side}`,
      ].join(" ")}
      aria-hidden="true"
      data-seat-index={seatIndex}
    >
      <img
        className="train-seat-sprite train-seat-sprite--empty"
        src={emptySeatUrl}
        alt=""
        draggable={false}
      />
    </span>
  );
}

function TrainLocomotiveMore({
  items,
  onSelect,
}: {
  items: PaneListItem[];
  onSelect: (paneID: string) => void;
}) {
  const [pos, setPos] = useState<{ left: number; bottom: number } | null>(null);
  const pop = useMenuPopover(pos);
  const buttonID = useId();
  const menuID = useId();

  useLayoutEffect(() => {
    if (!pop.open || !pop.buttonRef.current) return;
    const r = pop.buttonRef.current.getBoundingClientRect();
    setPos({
      left: Math.max(8, r.left),
      bottom: window.innerHeight - r.top + 8,
    });
  }, [pop.open]);

  const moreLabel =
    items.length > 0
      ? `Show ${items.length} more pane${items.length === 1 ? "" : "s"} and recently closed sessions`
      : "Show recently closed sessions";

  return (
    <>
      <button
        ref={pop.buttonRef}
        id={buttonID}
        type="button"
        className={`train-locomotive-more${pop.open ? " open" : ""}`}
        title={moreLabel}
        aria-label={moreLabel}
        aria-haspopup="menu"
        aria-expanded={pop.open}
        aria-controls={menuID}
        onPointerDown={onPointerDownNoBlur}
        onClick={pop.onTriggerClick}
        onKeyDown={pop.onTriggerKeyDown}
      >
        <img
          className="train-layout-locomotive"
          src={locomotiveUrl}
          alt=""
          draggable={false}
        />
      </button>
      {pop.open && pos
        ? createPortal(
            <div
              ref={pop.menuRef}
              id={menuID}
              className="train-more-pop ovf-pop"
              role="menu"
              aria-labelledby={buttonID}
              onKeyDown={pop.onMenuKeyDown}
              style={{
                position: "fixed",
                left: pos.left,
                bottom: pos.bottom,
                transform: "none",
              }}
            >
              <OverflowMenuContent
                items={items}
                onSelect={onSelect}
                closeRestoring={pop.closeRestoring}
              />
            </div>,
            document.body,
          )
        : null}
    </>
  );
}

type TrainRouteChunkStyle = CSSProperties & {
  "--train-chunk-terrain-height": string;
  "--train-chunk-ridge-height": string;
  "--train-chunk-feature-offset": string;
};

type TrainWorldLayerStyle = CSSProperties & {
  "--train-layer-order": number;
  "--train-layer-position": string;
  "--train-layer-speed": number;
};

type TrainSceneryAssetStyle = CSSProperties & {
  "--train-scenery-scale": number;
};

function TrainRouteChunk({
  chunk,
  layer,
}: {
  chunk: RouteChunk;
  layer: TrainParallaxLayer;
}) {
  const style: TrainRouteChunkStyle = {
    left: `${
      -chunk.index * TRAIN_ROUTE_CHUNK_WIDTH -
      TRAIN_PARALLAX_SEAM_OVERLAP / 2
    }px`,
    width: `${TRAIN_ROUTE_CHUNK_WIDTH + TRAIN_PARALLAX_SEAM_OVERLAP}px`,
    "--train-chunk-terrain-height": `${chunk.terrainHeight}px`,
    "--train-chunk-ridge-height": `${chunk.ridgeHeight}px`,
    "--train-chunk-feature-offset": `${chunk.featureOffset}%`,
  };
  const sceneryPlacements = trainSceneryPlacementsForChunk(layer.name, chunk);

  return (
    <div
      className={[
        "train-parallax-chunk",
        `train-parallax-chunk--${layer.name}`,
        `train-parallax-chunk--variant-${chunk.variant}`,
        layer.name === "near" ? "train-route-chunk" : "",
      ]
        .filter(Boolean)
        .join(" ")}
      data-route-chunk-index={chunk.index}
      data-route-chunk-variant={chunk.variant}
      data-route-region={chunk.region}
      data-route-region-index={chunk.regionIndex}
      data-route-region-offset={chunk.regionChunkOffset}
      data-parallax-layer={layer.name}
      data-seam-overlap={TRAIN_PARALLAX_SEAM_OVERLAP}
      style={style}
    >
      {sceneryPlacements.map((placement, ordinal) => {
        const { asset } = placement;
        const sceneryStyle: TrainSceneryAssetStyle = {
          left: `${placement.offsetPercent}%`,
          "--train-scenery-scale": placement.scale,
        };
        return (
          <img
            className={[
              "train-scenery-asset",
              `train-scenery-asset--${asset.category}`,
            ].join(" ")}
            src={asset.src}
            alt=""
            aria-hidden="true"
            draggable={false}
            width={asset.width}
            height={asset.height}
            data-scenery-asset={asset.id}
            data-scenery-category={asset.category}
            data-scenery-manifest-layer={asset.layer}
            data-scenery-anchor={asset.anchor}
            data-scenery-safe-scale={asset.safeScale.join("-")}
            data-scenery-day-night={asset.dayNightTreatment}
            data-scenery-landmark={placement.landmark ? "true" : "false"}
            data-scenery-collision-width={placement.collisionWidth.toFixed(3)}
            data-scenery-minimum-spacing={placement.minimumSpacingPx}
            style={sceneryStyle}
            key={`${asset.id}-${ordinal}`}
          />
        );
      })}
    </div>
  );
}

function initialWorldWidth(): number {
  return Math.max(1, window.innerWidth || TRAIN_ROUTE_CHUNK_WIDTH);
}

function sameRouteWindow(
  current: RouteChunkWindowSnapshot,
  next: Pick<RouteChunkWindowSnapshot, "firstIndex" | "lastIndex">,
): boolean {
  return current.firstIndex === next.firstIndex && current.lastIndex === next.lastIndex;
}

function routeChunkSlotKey(index: number, mountedCount: number): string {
  const slot = ((index % mountedCount) + mountedCount) % mountedCount;
  return `route-slot-${slot}`;
}

type TrainRouteEngines = Record<TrainParallaxLayerName, RouteChunkWindow>;
type TrainRouteWindows = Record<
  TrainParallaxLayerName,
  RouteChunkWindowSnapshot
>;

function createRouteEngines(seed: string): TrainRouteEngines {
  return Object.fromEntries(
    TRAIN_PARALLAX_LAYERS.map((layer) => [
      layer.name,
      new RouteChunkWindow(seed),
    ]),
  ) as TrainRouteEngines;
}

function createRouteWindows(
  routeEngines: TrainRouteEngines,
  viewportWidth: number,
): TrainRouteWindows {
  return Object.fromEntries(
    TRAIN_PARALLAX_LAYERS.map((layer) => [
      layer.name,
      routeEngines[layer.name].update(0, viewportWidth),
    ]),
  ) as TrainRouteWindows;
}

function prefersReducedTrainMotion(): boolean {
  return (
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

function TrainWorld() {
  const worldRef = useRef<HTMLDivElement | null>(null);
  const diagnosticsRef = useRef<HTMLOutputElement | null>(null);
  const debug = trainWorldDebugEnabled(window.location.search);
  const [reducedMotion] = useState(prefersReducedTrainMotion);
  const [routeEngines] = useState(() =>
    createRouteEngines(trainWorldRouteSeed(window.location.search)),
  );
  const seed = routeEngines.near.seed;
  const [routeWindows, setRouteWindows] = useState(() =>
    createRouteWindows(routeEngines, initialWorldWidth()),
  );
  const routeWindowsRef = useRef(routeWindows);
  routeWindowsRef.current = routeWindows;

  useEffect(() => {
    const world = worldRef.current;
    if (!world) return;

    let routePosition = 0;
    let previousTimestamp: number | null = null;
    let frame = 0;

    const viewportWidth = () => Math.max(1, world.clientWidth || initialWorldWidth());
    const applyRoutePosition = () => {
      const value = `${routePosition.toFixed(3)}px`;
      world.style.setProperty("--train-route-position", value);
      world.dataset.routePosition = value;
      const width = viewportWidth();
      let windowsChanged = false;
      const nextWindows = { ...routeWindowsRef.current };

      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const layerPosition = trainParallaxLayerPosition(
          routePosition,
          layer.speedRatio,
          reducedMotion,
        );
        const layerElement = world.querySelector<HTMLElement>(
          `[data-world-layer="${layer.name}"]`,
        );
        if (layerElement) {
          layerElement.style.setProperty(
            "--train-layer-position",
            `${layerPosition.toFixed(3)}px`,
          );
          layerElement.dataset.layerPosition = `${layerPosition.toFixed(3)}px`;
        }

        const routeEngine = routeEngines[layer.name];
        const nextRange = routeChunkWindowRange(
          layerPosition,
          width,
          routeEngine.chunkWidth,
          routeEngine.overscan,
        );
        const currentWindow = nextWindows[layer.name];
        if (!sameRouteWindow(currentWindow, nextRange)) {
          nextWindows[layer.name] = routeEngine.update(
            layerPosition,
            nextRange.viewportWidth,
          );
          windowsChanged = true;
        }
      }

      if (windowsChanged) {
        routeWindowsRef.current = nextWindows;
        setRouteWindows(nextWindows);
      }
      const nearWindow = nextWindows.near;
      const indices = nearWindow.chunks.map((chunk) => chunk.index).join(",");
      world.dataset.routeSeed = seed;
      world.dataset.routeSeedVersion = TRAIN_ROUTE_SEED_VERSION;
      world.dataset.routeChunkIndices = indices;
      world.dataset.routeMountedChunks = String(nearWindow.chunks.length);
      if (diagnosticsRef.current) {
        diagnosticsRef.current.value =
          `seed ${seed} · position ${routePosition.toFixed(1)}px · ` +
          `chunks ${indices} · mounted ${nearWindow.chunks.length}`;
      }
    };
    const advance = (timestamp: number) => {
      if (previousTimestamp !== null) {
        routePosition = advanceTrainWorldRoutePosition(
          routePosition,
          timestamp - previousTimestamp,
        );
        applyRoutePosition();
      }
      previousTimestamp = timestamp;
      frame = window.requestAnimationFrame(advance);
    };

    applyRoutePosition();
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(applyRoutePosition);
    observer?.observe(world);
    const handleResize = applyRoutePosition;
    window.addEventListener("resize", handleResize);
    if (!reducedMotion) frame = window.requestAnimationFrame(advance);
    return () => {
      if (frame) window.cancelAnimationFrame(frame);
      observer?.disconnect();
      window.removeEventListener("resize", handleResize);
    };
  }, [reducedMotion, routeEngines, seed]);

  const nearWindow = routeWindows.near;

  return (
    <div
      ref={worldRef}
      className="train-layout-world"
      aria-hidden="true"
      data-layer="world"
      data-route-direction="right"
      data-route-position="0.000px"
      data-route-seed={seed}
      data-route-seed-version={TRAIN_ROUTE_SEED_VERSION}
      data-route-chunk-indices={nearWindow.chunks
        .map((chunk) => chunk.index)
        .join(",")}
      data-route-mounted-chunks={nearWindow.chunks.length}
      data-motion={reducedMotion ? "reduced" : "full"}
    >
      {TRAIN_PARALLAX_LAYERS.map((layer, layerIndex) => {
        const layerWindow = routeWindows[layer.name];
        const style: TrainWorldLayerStyle = {
          "--train-layer-order": layerIndex,
          "--train-layer-position": "0.000px",
          "--train-layer-speed": layer.speedRatio,
        };
        return (
          <div
            className={`train-world-layer train-world-layer--${layer.name}`}
            data-world-layer={layer.name}
            data-layer-order={layerIndex}
            data-layer-position="0.000px"
            data-speed-ratio={layer.speedRatio}
            data-motion={reducedMotion ? "reduced" : "full"}
            style={style}
            key={layer.name}
          >
            <div className="train-world-layer-track">
              {layerWindow.chunks.map((chunk) => (
                <TrainRouteChunk
                  chunk={chunk}
                  layer={layer}
                  key={`${layer.name}-${routeChunkSlotKey(
                    chunk.index,
                    layerWindow.chunks.length,
                  )}`}
                />
              ))}
            </div>
          </div>
        );
      })}
      {debug ? (
        <div
          className="train-world-debug-grid"
          data-testid="train-world-debug-grid"
        >
          <span>world →</span>
          <output ref={diagnosticsRef} data-testid="train-route-diagnostics">
            seed {seed} · position 0.0px · chunks{" "}
            {nearWindow.chunks.map((chunk) => chunk.index).join(",")} · mounted{" "}
            {nearWindow.chunks.length}
          </output>
        </div>
      ) : null}
    </div>
  );
}

export function TrainLayout({ panes, selected, onSelect }: TrainLayoutProps) {
  const layoutRef = useRef<HTMLElement | null>(null);
  const [minimumCarriages, setMinimumCarriages] = useState(1);
  const items = paneListItems(panes);
  const { visible, overflow } = splitPaneItems(items, selected);
  const paneCarriageCount = Math.ceil(visible.length / 4);
  const carriages: PaneListItem[][] = [];
  const renderedCarriageCount = Math.max(paneCarriageCount, minimumCarriages);
  for (let carriageIndex = 0; carriageIndex < renderedCarriageCount; carriageIndex++) {
    const firstPassenger = carriageIndex * 4;
    carriages.push(visible.slice(firstPassenger, firstPassenger + 4));
  }

  useLayoutEffect(() => {
    const layout = layoutRef.current;
    if (!layout) return;

    const updateMinimum = () => {
      const locomotive = layout.querySelector<HTMLElement>(".train-locomotive-more");
      const carriage = layout.querySelector<HTMLElement>(".train-carriage");
      if (!locomotive || !carriage) return;
      const next = minimumCarriagesForWidth(
        layout.clientWidth,
        outerWidth(locomotive),
        outerWidth(carriage),
      );
      setMinimumCarriages((current) => (current === next ? current : next));
    };

    updateMinimum();
    // TrainLayout is lazy-loaded together with its CSS. On a cold load the
    // component can mount one frame before the stylesheet gives carriages a
    // measurable aspect-ratio/height, so repeat after paint and once shortly
    // afterward instead of waiting for a viewport resize.
    const frame = window.requestAnimationFrame(updateMinimum);
    const settleTimer = window.setTimeout(updateMinimum, 120);
    const observer =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(updateMinimum);
    observer?.observe(layout);
    window.addEventListener("resize", updateMinimum);
    return () => {
      window.cancelAnimationFrame(frame);
      window.clearTimeout(settleTimer);
      observer?.disconnect();
      window.removeEventListener("resize", updateMinimum);
    };
  }, []);

  return (
    <aside
      ref={layoutRef}
      className="train-layout"
      aria-label="Train pane switcher"
      data-minimum-carriages={minimumCarriages}
    >
      <TrainWorld />
      <div className="train-layout-inspection" data-layer="train">
        <div className="train-layout-scene">
          <div className="train-layout-consist">
            <TrainLocomotiveMore items={overflow} onSelect={onSelect} />
            {carriages.map((carriage, carriageIndex) => (
              <div
                className="train-carriage"
                role="group"
                aria-label={`Train carriage ${carriageIndex + 1}`}
                data-carriage-index={carriageIndex}
                data-filler-carriage={carriageIndex >= paneCarriageCount}
                key={`carriage-${carriageIndex}`}
              >
                <img
                  className="train-carriage-image"
                  src={carriageUrl}
                  alt=""
                  draggable={false}
                />
                {SEAT_CLASSES.map((_, seatIndex) => {
                  const item = carriage[seatIndex];
                  const globalIndex = carriageIndex * 4 + seatIndex;
                  return item ? (
                    <TrainPassenger
                      key={item.pane.pane_id || item.pane.target || globalIndex}
                      item={item}
                      globalIndex={globalIndex}
                      seatIndex={seatIndex}
                      selected={(item.pane.pane_id ?? "") === selected}
                      onSelect={onSelect}
                    />
                  ) : (
                    <EmptyTrainSeat
                      key={`empty-${carriageIndex}-${seatIndex}`}
                      seatIndex={seatIndex}
                    />
                  );
                })}
              </div>
            ))}
          </div>
          <div className="train-layout-track" aria-hidden="true" />
        </div>
      </div>
    </aside>
  );
}
