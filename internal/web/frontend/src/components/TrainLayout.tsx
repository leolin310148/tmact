// TrainLayout — train-theme pane switcher and bounded infinite journey.
//
// Panes use the same visible/overflow split as the chip and office switchers.
// Visible panes are packed four per double-decker carriage; overflow panes and
// recently-closed sessions live behind the locomotive. The carriage, empty
// chair, and occupied chair are separate aligned layers. Seat order is
// upper-left, upper-right, lower-left, lower-right; right-side chair sprites
// mirror the shared right-facing artwork so every pair faces inward.
// The scenery uses a separate, bounded route-chunk window behind the consist.

import {
  memo,
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
import {
  generateTrainStarCatalogue,
  type TrainStar,
} from "./trainStars";
import {
  SCENE_MODES,
  clockSceneMode,
  nextClockSceneModeBoundary,
  nextSceneMode,
  type SceneMode,
} from "./sceneTime";
import {
  trainWheelRotationDegrees,
  TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND,
  TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS,
} from "./trainMotion";
import {
  advanceTrainStationJourney,
  advanceTrainStationJourneyOnClock,
  createTrainStationJourney,
  TRAIN_STATION_DEFAULT_DWELL_MS,
  TRAIN_STATION_PLATFORM_SETTLE_MS,
  trainStationDevelopmentTrigger,
} from "./trainStation";
import "./TrainLayout.css";

export { advanceTrainWorldRoutePosition } from "./trainMotion";

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

const TRAIN_WORLD_DEBUG_PARAM = "train-world-debug";
const TRAIN_WORLD_SEED_PARAM = "train-route-seed";
const TRAIN_WORLD_SPEED_PARAM = "train-cruise-speed";
const TRAIN_PALETTE_TRANSITION_MS = 450;
const TRAIN_WORLD_TRACK_SPEED_RATIO = 1;
export const TRAIN_WORLD_TRACK_TILE_WIDTH = 240;
export const TRAIN_WORLD_TRACK_PERSPECTIVE = "shallow-three-quarter";
export const TRAIN_ARTWORK_SCALE = 0.9;
export const TRAIN_MIN_SEAT_TARGET_PX = 44;
export const TRAIN_LOCOMOTIVE_WHEEL_COUNT = 3;
export const TRAIN_CARRIAGE_WHEEL_COUNT = 4;

interface TrainWheelSpec {
  centerX: number;
  centerY: number;
  diameter: number;
}

// Coordinates are percentages of the unscaled sprite canvases. Keeping the
// rims in the same responsive container as each PNG preserves their measured
// centers through compact/desktop scaling and locomotive clipping.
const LOCOMOTIVE_WHEELS: readonly TrainWheelSpec[] = [
  { centerX: 23.88, centerY: 89.56, diameter: 19.58 },
  { centerX: 43.44, centerY: 85.12, diameter: 27.15 },
  { centerX: 60.5, centerY: 85.64, diameter: 25.59 },
];

const CARRIAGE_WHEELS: readonly TrainWheelSpec[] = [
  { centerX: 16.08, centerY: 93.47, diameter: 11.49 },
  { centerX: 30.94, centerY: 93.47, diameter: 11.49 },
  { centerX: 67.72, centerY: 93.47, diameter: 11.49 },
  { centerX: 83.43, centerY: 93.47, diameter: 11.49 },
];

interface TrainTimePalette {
  skyTop: string;
  skyBottom: string;
  haze: string;
  silhouette: string;
  farSurface: string;
  midSurface: string;
  nearSurface: string;
  water: string;
  foregroundContrast: string;
  controlSurface: string;
  emissive: string;
}

interface TrainSceneryDepthProfile {
  saturation: number;
  brightness: number;
  contrast: number;
}

interface TrainSceneryTimeGrade {
  saturation: number;
  brightness: number;
  warmth: number;
}

export const TRAIN_TIME_PALETTES: Readonly<Record<SceneMode, TrainTimePalette>> = {
  day: {
    skyTop: "#54a8d8",
    skyBottom: "#b9e4ef",
    haze: "rgba(194, 229, 239, 0.44)",
    silhouette: "#53767b",
    farSurface: "#426e64",
    midSurface: "#315c51",
    nearSurface: "#183f3b",
    water: "#4c9db5",
    foregroundContrast: "#10243a",
    controlSurface: "#f4fbff",
    emissive: "#fff2ad",
  },
  sunset: {
    skyTop: "#7b527a",
    skyBottom: "#e49a69",
    haze: "rgba(255, 190, 129, 0.42)",
    silhouette: "#59455d",
    farSurface: "#58465b",
    midSurface: "#463b50",
    nearSurface: "#2b3042",
    water: "#9a6173",
    foregroundContrast: "#fff6df",
    controlSurface: "#4b263f",
    emissive: "#ffd889",
  },
  night: {
    skyTop: "#09172b",
    skyBottom: "#102740",
    haze: "rgba(68, 101, 135, 0.25)",
    silhouette: "#142d47",
    farSurface: "#153752",
    midSurface: "#123149",
    nearSurface: "#0c2639",
    water: "#174b68",
    foregroundContrast: "#eaf6ff",
    controlSurface: "#07111f",
    emissive: "#ffe596",
  },
};

export const TRAIN_SCENERY_DEPTH_PROFILES: Readonly<
  Record<TrainParallaxLayerName, TrainSceneryDepthProfile>
> = {
  sky: { saturation: 0.76, brightness: 1.08, contrast: 0.7 },
  "ultra-far": { saturation: 0.72, brightness: 1.1, contrast: 0.72 },
  far: { saturation: 0.8, brightness: 1.04, contrast: 0.82 },
  midground: { saturation: 0.9, brightness: 0.98, contrast: 0.94 },
  near: { saturation: 1, brightness: 0.94, contrast: 1.06 },
};

export const TRAIN_SCENERY_TIME_GRADES: Readonly<
  Record<SceneMode, TrainSceneryTimeGrade>
> = {
  day: { saturation: 1, brightness: 1.06, warmth: 0 },
  sunset: { saturation: 0.94, brightness: 0.94, warmth: 0.12 },
  night: { saturation: 0.8, brightness: 0.78, warmth: 0 },
};

function channelLuminance(channel: number): number {
  const normalized = channel / 255;
  return normalized <= 0.04045
    ? normalized / 12.92
    : ((normalized + 0.055) / 1.055) ** 2.4;
}

function colorLuminance(hex: string): number {
  const channels = hex.match(/[a-f\d]{2}/gi)?.map((value) => Number.parseInt(value, 16));
  if (!channels || channels.length !== 3) return 0;
  return (
    0.2126 * channelLuminance(channels[0]!) +
    0.7152 * channelLuminance(channels[1]!) +
    0.0722 * channelLuminance(channels[2]!)
  );
}

export function trainPaletteContrastRatio(mode: SceneMode): number {
  const palette = TRAIN_TIME_PALETTES[mode];
  const foreground = colorLuminance(palette.foregroundContrast);
  const background = colorLuminance(palette.controlSurface);
  return (Math.max(foreground, background) + 0.05) /
    (Math.min(foreground, background) + 0.05);
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

export function trainWorldCruiseSpeed(search: string): number {
  if (!import.meta.env.DEV) return TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND;
  const requested = Number.parseFloat(
    new URLSearchParams(search).get(TRAIN_WORLD_SPEED_PARAM) ?? "",
  );
  if (!Number.isFinite(requested) || requested <= 0) {
    return TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND;
  }
  return Math.min(96, requested);
}

export function trainWorldTrackTransform(routePosition: number): number {
  if (!Number.isFinite(routePosition)) return 0;
  return (
    ((routePosition % TRAIN_WORLD_TRACK_TILE_WIDTH) +
      TRAIN_WORLD_TRACK_TILE_WIDTH) %
    TRAIN_WORLD_TRACK_TILE_WIDTH
  );
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
      data-min-hit-size={TRAIN_MIN_SEAT_TARGET_PX}
      onPointerDown={onPointerDownNoBlur}
      onClick={() => onSelect(paneID)}
    >
      <span className="train-seat-artwork" aria-hidden="true">
        {selected ? <span className="train-selected-set-aura" /> : null}
        <img
          className="train-seat-sprite train-seat-sprite--occupied"
          src={OCCUPIED_SEAT_URLS[characterIndex]}
          alt=""
          draggable={false}
        />
        {pane.asking && !pane.stale ? (
          <span className="train-person-ask">?</span>
        ) : null}
      </span>
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
      <span className="train-seat-artwork">
        <img
          className="train-seat-sprite train-seat-sprite--empty"
          src={emptySeatUrl}
          alt=""
          draggable={false}
        />
      </span>
    </span>
  );
}

type TrainWheelRimStyle = CSSProperties & {
  "--train-wheel-center-x": string;
  "--train-wheel-center-y": string;
  "--train-wheel-diameter": number;
};

function TrainWheelLayer({
  vehicle,
  wheels,
}: {
  vehicle: "locomotive" | "carriage";
  wheels: readonly TrainWheelSpec[];
}) {
  return (
    <span
      className={`train-wheel-layer train-wheel-layer--${vehicle}`}
      aria-hidden="true"
      data-wheel-layer={vehicle}
      data-wheel-count={wheels.length}
    >
      {wheels.map((wheel, wheelIndex) => {
        const style: TrainWheelRimStyle = {
          "--train-wheel-center-x": `${wheel.centerX}%`,
          "--train-wheel-center-y": `${wheel.centerY}%`,
          "--train-wheel-diameter": wheel.diameter,
        };
        return (
          <span
            className="train-wheel-rim"
            data-wheel-rim={vehicle}
            data-wheel-index={wheelIndex}
            data-wheel-center={`${wheel.centerX},${wheel.centerY}`}
            style={style}
            key={`${vehicle}-wheel-${wheelIndex}`}
          />
        );
      })}
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
        <TrainWheelLayer vehicle="locomotive" wheels={LOCOMOTIVE_WHEELS} />
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
  "--train-depth-saturation": number;
  "--train-depth-brightness": number;
  "--train-depth-contrast": number;
};

type TrainSceneryAssetStyle = CSSProperties & {
  "--train-scenery-scale": number;
};

type TrainPaletteStyle = CSSProperties & {
  "--train-palette-sky-top": string;
  "--train-palette-sky-bottom": string;
  "--train-palette-haze": string;
  "--train-palette-silhouette": string;
  "--train-palette-far-surface": string;
  "--train-palette-mid-surface": string;
  "--train-palette-near-surface": string;
  "--train-palette-water": string;
  "--train-palette-foreground-contrast": string;
  "--train-palette-control-surface": string;
  "--train-palette-emissive": string;
  "--train-time-scenery-saturation": number;
  "--train-time-scenery-brightness": number;
  "--train-time-scenery-warmth": number;
};

type TrainAtmosphereStyle = CSSProperties & {
  "--train-atmosphere-sky-top": string;
  "--train-atmosphere-sky-bottom": string;
};

type TrainDepthVeilPaletteStyle = CSSProperties & {
  "--train-depth-veil-color": string;
};

type TrainStarStyle = CSSProperties & {
  "--train-star-brightness": number;
  "--train-star-size": string;
};

function trainPaletteStyle(mode: SceneMode): TrainPaletteStyle {
  const palette = TRAIN_TIME_PALETTES[mode];
  const grade = TRAIN_SCENERY_TIME_GRADES[mode];
  return {
    "--train-palette-sky-top": palette.skyTop,
    "--train-palette-sky-bottom": palette.skyBottom,
    "--train-palette-haze": palette.haze,
    "--train-palette-silhouette": palette.silhouette,
    "--train-palette-far-surface": palette.farSurface,
    "--train-palette-mid-surface": palette.midSurface,
    "--train-palette-near-surface": palette.nearSurface,
    "--train-palette-water": palette.water,
    "--train-palette-foreground-contrast": palette.foregroundContrast,
    "--train-palette-control-surface": palette.controlSurface,
    "--train-palette-emissive": palette.emissive,
    "--train-time-scenery-saturation": grade.saturation,
    "--train-time-scenery-brightness": grade.brightness,
    "--train-time-scenery-warmth": grade.warmth,
  };
}

function trainAtmosphereStyle(mode: SceneMode): TrainAtmosphereStyle {
  const palette = TRAIN_TIME_PALETTES[mode];
  return {
    "--train-atmosphere-sky-top": palette.skyTop,
    "--train-atmosphere-sky-bottom": palette.skyBottom,
  };
}

export const TrainRouteChunk = memo(function TrainRouteChunk({
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
  const stationSegment =
    chunk.setPiece?.type === "station" ? chunk.setPiece : null;

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
      data-route-set-piece={chunk.setPiece?.type ?? "none"}
      data-route-set-piece-role={chunk.setPiece?.role ?? "none"}
      data-route-set-piece-reserved-layers={
        chunk.setPiece?.reservedLayers.join(",") ?? ""
      }
      data-parallax-layer={layer.name}
      data-seam-overlap={TRAIN_PARALLAX_SEAM_OVERLAP}
      style={style}
    >
      {chunk.setPiece?.renderLayer === layer.name ? (
        <>
          <span
            className={[
              "train-set-piece",
              `train-set-piece--${chunk.setPiece.type}`,
              `train-set-piece--${chunk.setPiece.role}`,
            ].join(" ")}
            data-set-piece-id={chunk.setPiece.id}
            data-set-piece-type={chunk.setPiece.type}
            data-set-piece-role={chunk.setPiece.role}
            data-set-piece-segment={chunk.setPiece.segmentOffset}
            data-set-piece-span={chunk.setPiece.span}
            data-set-piece-start={chunk.setPiece.startIndex}
            data-set-piece-end={chunk.setPiece.endIndex}
            data-set-piece-occlusion="restrained"
            data-station-assets={
              stationSegment
                ? "platform,building,canopy,lamps"
                : undefined
            }
            data-station-vertical-zone={
              stationSegment ? "behind-train" : undefined
            }
          >
            {stationSegment ? (
              <>
                <span
                  className="train-station-platform"
                  data-station-asset="platform"
                />
                <span
                  className="train-station-building"
                  data-station-asset="building"
                >
                  <span
                    className="train-station-window-row"
                    data-station-asset="windows"
                  />
                  {stationSegment.segmentOffset === 2 ? (
                    <span
                      className="train-station-name-board"
                      data-station-asset="sign"
                    >
                      TMACT
                    </span>
                  ) : null}
                </span>
                <span
                  className="train-station-canopy"
                  data-station-asset="canopy"
                />
                <span
                  className="train-station-lamp train-station-lamp--leading"
                  data-station-asset="lamp"
                />
                <span
                  className="train-station-lamp train-station-lamp--trailing"
                  data-station-asset="lamp"
                />
              </>
            ) : null}
          </span>
          {stationSegment &&
          stationSegment.segmentOffset >= 1 &&
          stationSegment.segmentOffset <= 4 ? (
            <>
              <span
                className="train-station-signal"
                data-station-asset="signal"
                data-station-signal-aspect={
                  stationSegment.segmentOffset >= 3 ? "proceed" : "approach"
                }
              />
              {stationSegment.segmentOffset === 2 ? (
                <span
                  className="train-station-ambient-steam"
                  data-station-ambient-detail="steam"
                />
              ) : null}
            </>
          ) : null}
        </>
      ) : null}
      {sceneryPlacements.map((placement, ordinal) => {
        const { asset } = placement;
        const sceneryStyle: TrainSceneryAssetStyle = {
          left: `${placement.offsetPercent}%`,
          top:
            placement.altitudePercent === undefined
              ? undefined
              : `${placement.altitudePercent}%`,
          "--train-scenery-scale": placement.scale,
        };
        const sprites = [
          <img
            className={[
              "train-scenery-asset",
              `train-scenery-asset--${asset.category}`,
            ].join(" ")}
            src={asset.src}
            alt=""
            aria-hidden="true"
            draggable={false}
            loading="lazy"
            decoding="async"
            width={asset.width}
            height={asset.height}
            data-scenery-asset={asset.id}
            data-scenery-category={asset.category}
            data-scenery-manifest-layer={asset.layer}
            data-scenery-anchor={asset.anchor}
            data-scenery-safe-scale={asset.safeScale.join("-")}
            data-scenery-day-night={asset.dayNightTreatment}
            data-scenery-landmark={placement.landmark ? "true" : "false"}
            data-scenery-set-piece={placement.setPiece?.type ?? "none"}
            data-scenery-set-piece-role={placement.setPiece?.role ?? "none"}
            data-scenery-collision-width={placement.collisionWidth.toFixed(3)}
            data-scenery-minimum-spacing={placement.minimumSpacingPx}
            data-cloud-altitude={
              placement.altitudePercent?.toFixed(3) ?? undefined
            }
            data-cloud-pattern={placement.cloudPattern}
            data-cloud-group={placement.cloudGroup || undefined}
            data-cloud-route-position={
              placement.routePositionPx?.toFixed(3) ?? undefined
            }
            style={sceneryStyle}
            key={`base-${asset.id}-${ordinal}`}
          />,
        ];
        if (asset.emissive) {
          sprites.push(
            <img
              className="train-emissive-overlay train-scenery-emissive-mask"
              src={asset.emissive.src}
              alt=""
              aria-hidden="true"
              draggable={false}
              loading="lazy"
              decoding="async"
              width={asset.emissive.width}
              height={asset.emissive.height}
              data-emissive="building-windows"
              data-emissive-kind={asset.emissive.kind}
              data-emissive-owner={asset.id}
              data-emissive-load="pending"
              data-scenery-anchor={asset.anchor}
              data-scenery-manifest-layer={asset.layer}
              onLoad={(event) => {
                event.currentTarget.dataset.emissiveLoad = "loaded";
              }}
              onError={(event) => {
                event.currentTarget.dataset.emissiveLoad = "failed";
                event.currentTarget.hidden = true;
              }}
              style={sceneryStyle}
              key={`emissive-${asset.id}-${ordinal}`}
            />,
          );
        }
        return sprites;
      })}
      {sceneryPlacements
        .filter(
          (placement) =>
            placement.asset.category === "building" &&
            !placement.asset.emissive,
        )
        .map((placement, ordinal) => (
          <span
            className="train-emissive-overlay train-emissive-overlay--windows"
            data-emissive="windows"
            style={{ left: `${placement.offsetPercent}%` }}
            key={`windows-${placement.asset.id}-${ordinal}`}
          />
        ))}
      {layer.name === "midground" &&
      chunk.setPiece?.type === "town-edge" ? (
        <span
          className="train-emissive-overlay train-emissive-overlay--windows train-emissive-overlay--town-edge-windows"
          data-emissive="windows"
          data-set-piece-id={chunk.setPiece.id}
        />
      ) : null}
      {layer.name === "midground" ? (
        <span
          className="train-emissive-overlay train-emissive-overlay--streetlight"
          data-emissive="streetlight"
          data-emissive-enabled={
            chunk.region === "town" || chunk.region === "industrial"
              ? "true"
              : "false"
          }
          style={{ left: `${18 + chunk.variant * 14}%` }}
        />
      ) : null}
      {layer.name === "near" ? (
        <span
          className="train-emissive-overlay train-emissive-overlay--station-lamp"
          data-emissive="station-lamp"
          data-emissive-enabled={
            chunk.region === "town" && chunk.regionChunkOffset === 0
              ? "true"
              : "false"
          }
          style={{ left: `${24 + chunk.variant * 11}%` }}
        />
      ) : null}
      {layer.name === "near" ? (
        <span
          className="train-emissive-overlay train-emissive-overlay--signal"
          data-emissive="signal"
          data-emissive-enabled={
            chunk.region === "town" || chunk.region === "industrial"
              ? "true"
              : "false"
          }
          style={{ left: `${78 - chunk.variant * 9}%` }}
        />
      ) : null}
      {layer.name === "far" ? (
        <span
          className="train-emissive-overlay train-emissive-overlay--water-reflection"
          data-emissive="water-reflection"
          data-emissive-enabled={chunk.region === "coast" ? "true" : "false"}
        />
      ) : null}
    </div>
  );
});

function initialWorldWidth(): number {
  return Math.max(1, window.innerWidth || TRAIN_ROUTE_CHUNK_WIDTH);
}

function sameRouteWindow(
  current: RouteChunkWindowSnapshot,
  next: Pick<
    RouteChunkWindowSnapshot,
    "firstIndex" | "lastIndex" | "viewportWidth"
  >,
): boolean {
  return (
    current.firstIndex === next.firstIndex &&
    current.lastIndex === next.lastIndex &&
    current.viewportWidth === next.viewportWidth
  );
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

function totalMountedRouteChunks(routeWindows: TrainRouteWindows): number {
  return TRAIN_PARALLAX_LAYERS.reduce(
    (total, layer) => total + routeWindows[layer.name].chunks.length,
    0,
  );
}

function usePrefersReducedTrainMotion(): boolean {
  const [reducedMotion, setReducedMotion] = useState(
    () =>
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches,
  );

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(query.matches);
    query.addEventListener?.("change", update);
    return () => query.removeEventListener?.("change", update);
  }, []);

  return reducedMotion;
}

function useClockSceneMode(): SceneMode {
  const [mode, setMode] = useState(() => clockSceneMode(new Date()));

  useEffect(() => {
    let timer: number | null = null;

    const updateAndSchedule = () => {
      const now = new Date();
      setMode(clockSceneMode(now));
      const nextBoundary = nextClockSceneModeBoundary(now);
      timer = window.setTimeout(
        updateAndSchedule,
        Math.max(1, nextBoundary.getTime() - now.getTime()),
      );
    };

    updateAndSchedule();
    return () => {
      if (timer !== null) window.clearTimeout(timer);
    };
  }, []);

  return mode;
}

function TrainWorld({
  timeOfDay,
  timeSource,
  paletteTransition,
}: {
  timeOfDay: SceneMode;
  timeSource: "clock" | "manual";
  paletteTransition: "settled" | "crossfading";
}) {
  const worldRef = useRef<HTMLDivElement | null>(null);
  const diagnosticsRef = useRef<HTMLOutputElement | null>(null);
  const debug = trainWorldDebugEnabled(window.location.search);
  const reducedMotion = usePrefersReducedTrainMotion();
  const [cruiseSpeed] = useState(() =>
    trainWorldCruiseSpeed(window.location.search),
  );
  const [routeEngines] = useState(() =>
    createRouteEngines(trainWorldRouteSeed(window.location.search)),
  );
  const seed = routeEngines.near.seed;
  const [initialStationJourney] = useState(() =>
    createTrainStationJourney(
      seed,
      0,
      { cruiseSpeed },
      trainStationDevelopmentTrigger(window.location.search),
    ),
  );
  const [routeWindows, setRouteWindows] = useState(() =>
    Object.fromEntries(
      TRAIN_PARALLAX_LAYERS.map((layer) => [
        layer.name,
        routeEngines[layer.name].update(
          trainParallaxLayerPosition(
            initialStationJourney.routePosition,
            layer.speedRatio,
          ),
          initialWorldWidth(),
        ),
      ]),
    ) as TrainRouteWindows,
  );
  const [starCatalogue, setStarCatalogue] = useState(() =>
    generateTrainStarCatalogue(seed, initialWorldWidth()),
  );
  const starViewportWidthRef = useRef(starCatalogue.viewportWidth);
  const routeWindowsRef = useRef(routeWindows);
  const stationJourneyRef = useRef(initialStationJourney);
  const routePositionRef = useRef(initialStationJourney.routePosition);
  routeWindowsRef.current = routeWindows;

  useEffect(() => {
    const world = worldRef.current;
    if (!world) return;

    let previousTimestamp: number | null = null;
    let frame: number | null = null;
    let reducedTimer: number | null = null;
    let reducedClockStartedAt: number | null = null;
    let active = true;
    let routeApplyCount = 0;
    let routeWindowUpdateCount = 0;

    const viewportWidth = () => Math.max(1, world.clientWidth || initialWorldWidth());
    const applyRoutePosition = () => {
      const routePosition = routePositionRef.current;
      routeApplyCount += 1;
      const value = `${routePosition.toFixed(3)}px`;
      world.style.setProperty("--train-route-position", value);
      world.dataset.routePosition = value;
      world.dataset.routeApplyCount = String(routeApplyCount);
      const layout = world.closest<HTMLElement>(".train-layout");
      const wheelRotation =
        `${trainWheelRotationDegrees(routePosition).toFixed(3)}deg`;
      if (layout) {
        layout.style.setProperty("--train-wheel-rotation", wheelRotation);
        layout.dataset.wheelRotation = wheelRotation;
      }
      const stationJourney = stationJourneyRef.current;
      world.dataset.stationState = stationJourney.state;
      world.dataset.stationTargetSpeed =
        stationJourney.targetSpeed.toFixed(3);
      world.dataset.stationCurrentSpeed =
        stationJourney.currentSpeed.toFixed(3);
      world.dataset.stationEventId = stationJourney.station.id;
      world.dataset.stationStopPosition =
        `${stationJourney.station.stopPosition.toFixed(3)}px`;
      world.dataset.stationPositionalMotion =
        stationJourney.state === "platform" ||
        stationJourney.state === "dwell"
          ? "stopped"
          : "moving";
      world.dataset.stationAmbient =
        stationJourney.state === "dwell" ? "running" : "available";
      const trackPosition = trainParallaxLayerPosition(
        routePosition,
        TRAIN_WORLD_TRACK_SPEED_RATIO,
      );
      const track = world.querySelector<HTMLElement>(
        '[data-world-track="railway"]',
      );
      if (track) {
        const trackPositionValue = `${trackPosition.toFixed(3)}px`;
        const trackTransformValue =
          `${trainWorldTrackTransform(trackPosition).toFixed(3)}px`;
        track.style.setProperty(
          "--train-track-transform",
          trackTransformValue,
        );
        track.dataset.trackPosition = trackPositionValue;
        track.dataset.trackTransform = trackTransformValue;
      }
      const width = viewportWidth();
      const starViewportWidth = Math.max(1, Math.round(width));
      if (starViewportWidthRef.current !== starViewportWidth) {
        starViewportWidthRef.current = starViewportWidth;
        setStarCatalogue(generateTrainStarCatalogue(seed, starViewportWidth));
      }
      let windowsChanged = false;
      const nextWindows = { ...routeWindowsRef.current };

      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const layerPosition = trainParallaxLayerPosition(
          routePosition,
          layer.speedRatio,
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
        routeWindowUpdateCount += 1;
        routeWindowsRef.current = nextWindows;
        setRouteWindows(nextWindows);
      }
      world.dataset.routeWindowUpdates = String(routeWindowUpdateCount);
      const nearWindow = nextWindows.near;
      const indices = nearWindow.chunks.map((chunk) => chunk.index).join(",");
      world.dataset.routeSeed = seed;
      world.dataset.routeSeedVersion = TRAIN_ROUTE_SEED_VERSION;
      world.dataset.routeChunkIndices = indices;
      world.dataset.routeMountedChunks = String(nearWindow.chunks.length);
      world.dataset.routeTotalMountedChunks = String(
        totalMountedRouteChunks(nextWindows),
      );
      if (diagnosticsRef.current) {
        diagnosticsRef.current.value =
          `seed ${seed} · position ${routePosition.toFixed(1)}px · ` +
          `chunks ${indices} · near ${nearWindow.chunks.length} · ` +
          `total ${totalMountedRouteChunks(nextWindows)} · ` +
          `station ${stationJourney.state} → ${stationJourney.station.id}`;
      }
    };

    const documentIsHidden = () => document.visibilityState === "hidden";
    const setMotionState = (state: "running" | "suspended") => {
      world.dataset.motionState = state;
      const layout = world.closest<HTMLElement>(".train-layout");
      if (layout) layout.dataset.wheelMotionState = state;
      const track = world.querySelector<HTMLElement>(
        '[data-world-track="railway"]',
      );
      if (track) track.dataset.motionState = state;
    };
    const cancelScheduledMotion = () => {
      if (frame !== null) {
        window.cancelAnimationFrame(frame);
        frame = null;
      }
      if (reducedTimer !== null) {
        window.clearTimeout(reducedTimer);
        reducedTimer = null;
      }
      reducedClockStartedAt = null;
    };
    const reducedPhaseDuration = () => {
      const journey = stationJourneyRef.current;
      if (journey.state === "platform") {
        return TRAIN_STATION_PLATFORM_SETTLE_MS;
      }
      if (journey.state === "dwell") {
        return TRAIN_STATION_DEFAULT_DWELL_MS;
      }
      return null;
    };
    const advanceReducedStationClock = (timestamp: number) => {
      const phaseDuration = reducedPhaseDuration();
      if (phaseDuration === null || reducedClockStartedAt === null) return;
      const elapsed = Math.max(0, timestamp - reducedClockStartedAt);
      if (elapsed === 0) return;
      stationJourneyRef.current = advanceTrainStationJourneyOnClock(
        stationJourneyRef.current,
        0,
        elapsed,
        { cruiseSpeed },
      );
      routePositionRef.current = stationJourneyRef.current.routePosition;
      reducedClockStartedAt = timestamp;
      applyRoutePosition();
    };
    const scheduleMotion = () => {
      if (
        !active ||
        documentIsHidden() ||
        frame !== null ||
        reducedTimer !== null
      ) {
        return;
      }
      setMotionState("running");
      if (reducedMotion) {
        const phaseDuration = reducedPhaseDuration();
        const delay =
          phaseDuration === null
            ? TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS
            : Math.max(
                0,
                phaseDuration - stationJourneyRef.current.stateElapsedMs,
              );
        reducedClockStartedAt = Date.now();
        reducedTimer = window.setTimeout(() => {
          reducedTimer = null;
          if (!active || documentIsHidden()) return;
          const timestamp = Date.now();
          if (phaseDuration === null) {
            stationJourneyRef.current = advanceTrainStationJourneyOnClock(
              stationJourneyRef.current,
              TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS,
              0,
              { cruiseSpeed },
            );
          } else {
            advanceReducedStationClock(timestamp);
          }
          routePositionRef.current =
            stationJourneyRef.current.routePosition;
          reducedClockStartedAt = null;
          if (phaseDuration === null) applyRoutePosition();
          scheduleMotion();
        }, delay);
        return;
      }
      frame = window.requestAnimationFrame(advance);
    };
    const advance = (timestamp: number) => {
      frame = null;
      if (!active || documentIsHidden()) return;
      if (previousTimestamp !== null) {
        stationJourneyRef.current = advanceTrainStationJourney(
          stationJourneyRef.current,
          timestamp - previousTimestamp,
          { cruiseSpeed },
        );
        routePositionRef.current = stationJourneyRef.current.routePosition;
        applyRoutePosition();
      }
      previousTimestamp = timestamp;
      scheduleMotion();
    };
    const handleVisibility = () => {
      if (
        reducedMotion &&
        documentIsHidden() &&
        reducedClockStartedAt !== null
      ) {
        advanceReducedStationClock(Date.now());
      }
      cancelScheduledMotion();
      previousTimestamp = null;
      if (documentIsHidden()) {
        setMotionState("suspended");
        world.dataset.stationAmbient = "suspended";
        return;
      }
      scheduleMotion();
    };

    applyRoutePosition();
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(applyRoutePosition);
    observer?.observe(world);
    const handleResize = applyRoutePosition;
    window.addEventListener("resize", handleResize);
    document.addEventListener("visibilitychange", handleVisibility);
    if (documentIsHidden()) {
      setMotionState("suspended");
    } else {
      scheduleMotion();
    }
    return () => {
      active = false;
      cancelScheduledMotion();
      observer?.disconnect();
      window.removeEventListener("resize", handleResize);
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [cruiseSpeed, reducedMotion, routeEngines, seed]);

  const nearWindow = routeWindows.near;

  return (
    <div
      ref={worldRef}
      className="train-layout-world"
      aria-hidden="true"
      data-layer="world"
      data-route-direction="right"
      data-route-position={`${initialStationJourney.routePosition.toFixed(3)}px`}
      data-route-seed={seed}
      data-route-seed-version={TRAIN_ROUTE_SEED_VERSION}
      data-route-chunk-indices={nearWindow.chunks
        .map((chunk) => chunk.index)
        .join(",")}
      data-route-mounted-chunks={nearWindow.chunks.length}
      data-route-total-mounted-chunks={totalMountedRouteChunks(routeWindows)}
      data-motion={reducedMotion ? "reduced" : "full"}
      data-motion-state={
        document.visibilityState === "hidden" ? "suspended" : "running"
      }
      data-cruise-speed={cruiseSpeed}
      data-station-state={initialStationJourney.state}
      data-station-target-speed={initialStationJourney.targetSpeed.toFixed(3)}
      data-station-current-speed={initialStationJourney.currentSpeed.toFixed(3)}
      data-station-event-id={initialStationJourney.station.id}
      data-station-stop-position={`${initialStationJourney.station.stopPosition.toFixed(3)}px`}
      data-station-positional-motion="moving"
      data-station-ambient="available"
      data-route-apply-count="0"
      data-route-window-updates="0"
      data-star-count={starCatalogue.stars.length}
      data-star-viewport-width={starCatalogue.viewportWidth}
      data-time-of-day={timeOfDay}
      data-time-source={timeSource}
      data-palette-transition={paletteTransition}
      style={trainPaletteStyle(timeOfDay)}
    >
      <div className="train-world-atmospheres">
        {SCENE_MODES.map((mode) => (
          <span
            className={[
              "train-world-atmosphere",
              `train-world-atmosphere--${mode}`,
              mode === timeOfDay ? "is-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            data-atmosphere={mode}
            style={trainAtmosphereStyle(mode)}
            key={mode}
          />
        ))}
      </div>
      <div
        className="train-sky-emissive"
        data-star-count={starCatalogue.stars.length}
        data-star-viewport-width={starCatalogue.viewportWidth}
        aria-hidden="true"
      >
        <span
          className="train-emissive-overlay train-emissive-overlay--stars"
          data-emissive="stars"
          data-star-catalogue={starCatalogue.seed}
          data-star-count={starCatalogue.stars.length}
          data-star-target-count={starCatalogue.targetCount}
          data-star-negative-space={`${starCatalogue.negativeSpaceStartPercent.toFixed(
            3,
          )}-${starCatalogue.negativeSpaceEndPercent.toFixed(3)}`}
          data-motion={reducedMotion ? "reduced" : "full"}
        >
          {starCatalogue.stars.map((star: TrainStar) => {
            const style: TrainStarStyle = {
              left: `${star.xPercent}%`,
              top: `${star.yPercent}%`,
              "--train-star-size": `${star.sizePx.toFixed(3)}px`,
              "--train-star-brightness": star.brightness,
            };
            return (
              <i
                className={[
                  "train-star",
                  `train-star--${star.tint}`,
                  `train-star--${star.intensity}`,
                ].join(" ")}
                data-star-id={star.id}
                data-star-tint={star.tint}
                data-star-intensity={star.intensity}
                data-star-group={star.group ?? ""}
                style={style}
                key={star.id}
              />
            );
          })}
        </span>
        <span
          className="train-emissive-overlay train-emissive-overlay--moon"
          data-emissive="moon"
        />
      </div>
      {TRAIN_PARALLAX_LAYERS.map((layer, layerIndex) => {
        const layerWindow = routeWindows[layer.name];
        const depth = TRAIN_SCENERY_DEPTH_PROFILES[layer.name];
        const style: TrainWorldLayerStyle = {
          "--train-layer-order": layerIndex,
          "--train-layer-position": "0.000px",
          "--train-layer-speed": layer.speedRatio,
          "--train-depth-saturation": depth.saturation,
          "--train-depth-brightness": depth.brightness,
          "--train-depth-contrast": depth.contrast,
        };
        return (
          <div
            className={`train-world-layer train-world-layer--${layer.name}`}
            data-world-layer={layer.name}
            data-layer-order={layerIndex}
            data-layer-position="0.000px"
            data-speed-ratio={layer.speedRatio}
            data-depth-saturation={depth.saturation}
            data-depth-brightness={depth.brightness}
            data-depth-contrast={depth.contrast}
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
      <span
        className="train-depth-veil train-depth-veil--ultra-far"
        data-depth-veil="ultra-far"
        data-atmosphere-owner="depth-compositor"
        data-between-layers="ultra-far,far"
      >
        {SCENE_MODES.map((mode) => (
          <i
            className={[
              "train-depth-veil-palette",
              mode === timeOfDay ? "is-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            data-depth-veil-palette={mode}
            style={
              {
                "--train-depth-veil-color": TRAIN_TIME_PALETTES[mode].haze,
              } as TrainDepthVeilPaletteStyle
            }
            key={mode}
          />
        ))}
      </span>
      <span
        className="train-depth-veil train-depth-veil--far"
        data-depth-veil="far"
        data-atmosphere-owner="depth-compositor"
        data-between-layers="far,midground"
      >
        {SCENE_MODES.map((mode) => (
          <i
            className={[
              "train-depth-veil-palette",
              mode === timeOfDay ? "is-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            data-depth-veil-palette={mode}
            style={
              {
                "--train-depth-veil-color": TRAIN_TIME_PALETTES[mode].haze,
              } as TrainDepthVeilPaletteStyle
            }
            key={mode}
          />
        ))}
      </span>
      <div
        className="train-world-track"
        data-world-track="railway"
        data-track-perspective={TRAIN_WORLD_TRACK_PERSPECTIVE}
        data-track-tile-width={TRAIN_WORLD_TRACK_TILE_WIDTH}
        data-route-direction="right"
        data-speed-ratio={TRAIN_WORLD_TRACK_SPEED_RATIO}
        data-track-position={`${initialStationJourney.routePosition.toFixed(3)}px`}
        data-track-transform={`${trainWorldTrackTransform(
          initialStationJourney.routePosition,
        ).toFixed(3)}px`}
        data-motion={reducedMotion ? "reduced" : "full"}
        data-motion-state={
          document.visibilityState === "hidden" ? "suspended" : "running"
        }
      />
      {debug ? (
        <div
          className="train-world-debug-grid"
          data-testid="train-world-debug-grid"
        >
          <span>world →</span>
          <output ref={diagnosticsRef} data-testid="train-route-diagnostics">
            seed {seed} · position 0.0px · chunks{" "}
            {nearWindow.chunks.map((chunk) => chunk.index).join(",")} · near{" "}
            {nearWindow.chunks.length} · total{" "}
            {totalMountedRouteChunks(routeWindows)}
          </output>
        </div>
      ) : null}
    </div>
  );
}

export function TrainLayout({ panes, selected, onSelect }: TrainLayoutProps) {
  const layoutRef = useRef<HTMLElement | null>(null);
  const [minimumCarriages, setMinimumCarriages] = useState(1);
  const [modeOverride, setModeOverride] = useState<SceneMode | null>(null);
  const clockMode = useClockSceneMode();
  const timeOfDay = modeOverride ?? clockMode;
  const [paletteTransition, setPaletteTransition] = useState<
    "settled" | "crossfading"
  >("settled");
  const previousTimeOfDay = useRef(timeOfDay);
  const items = paneListItems(panes);
  const { visible, overflow } = splitPaneItems(items, selected);
  const paneCarriageCount = Math.ceil(visible.length / 4);
  const carriages: PaneListItem[][] = [];
  const renderedCarriageCount = Math.max(paneCarriageCount, minimumCarriages);
  for (let carriageIndex = 0; carriageIndex < renderedCarriageCount; carriageIndex++) {
    const firstPassenger = carriageIndex * 4;
    carriages.push(visible.slice(firstPassenger, firstPassenger + 4));
  }

  useEffect(() => {
    if (previousTimeOfDay.current === timeOfDay) return;
    previousTimeOfDay.current = timeOfDay;
    setPaletteTransition("crossfading");
    const transitionTimer = window.setTimeout(
      () => setPaletteTransition("settled"),
      TRAIN_PALETTE_TRANSITION_MS,
    );
    return () => window.clearTimeout(transitionTimer);
  }, [timeOfDay]);

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
      data-artwork-scale={TRAIN_ARTWORK_SCALE}
      data-minimum-carriages={minimumCarriages}
      data-wheel-node-count={
        TRAIN_LOCOMOTIVE_WHEEL_COUNT +
        renderedCarriageCount * TRAIN_CARRIAGE_WHEEL_COUNT
      }
      data-wheel-motion-state={
        document.visibilityState === "hidden" ? "suspended" : "running"
      }
      style={trainPaletteStyle(timeOfDay)}
    >
      <TrainWorld
        timeOfDay={timeOfDay}
        timeSource={modeOverride === null ? "clock" : "manual"}
        paletteTransition={paletteTransition}
      />
      <div className="train-layout-inspection" data-layer="train">
        <div className="train-layout-scene">
          <div className="train-layout-consist">
            <TrainLocomotiveMore items={overflow} onSelect={onSelect} />
            {carriages.map((carriage, carriageIndex) => (
              <div
                className="train-carriage"
                role="group"
                aria-label={`Train carriage ${carriageIndex + 1}`}
                data-artwork-scale={TRAIN_ARTWORK_SCALE}
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
                <TrainWheelLayer vehicle="carriage" wheels={CARRIAGE_WHEELS} />
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
        </div>
      </div>
      <button
        type="button"
        className="train-time-toggle"
        aria-label="Cycle train lighting (day / sunset / night)"
        title={`Train lighting: ${timeOfDay}. Cycle day / sunset / night`}
        data-time-of-day={timeOfDay}
        data-time-source={modeOverride === null ? "clock" : "manual"}
        onPointerDown={onPointerDownNoBlur}
        onClick={() => setModeOverride(nextSceneMode(timeOfDay))}
      >
        <span aria-hidden="true">
          {timeOfDay === "day" ? "☀" : timeOfDay === "sunset" ? "◐" : "☾"}
        </span>
      </button>
    </aside>
  );
}
