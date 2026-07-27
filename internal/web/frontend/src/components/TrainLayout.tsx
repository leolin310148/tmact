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
  generateRouteChunk,
  RouteChunkWindow,
  routeChunkWindowRange,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_PARALLAX_SEAM_OVERLAP,
  TRAIN_ROUTE_CHUNK_WIDTH,
  TRAIN_ROUTE_SEED_VERSION,
  TRAIN_SET_PIECE_DEFINITIONS,
  trainSetPieceFocusForOccurrence,
  trainSetPieceFocusFromSegment,
  trainSetPieceProjectedCoordinate,
  trainSetPieceReservationIntersectsChunk,
  trainSetPieceScreenGeometry,
  trainSetPiecesAreIncompatible,
  trainParallaxLayerPosition,
  type RouteChunk,
  type RouteChunkWindowSnapshot,
  type TrainParallaxLayer,
  type TrainParallaxLayerName,
  type TrainSetPieceFocus,
  type TrainSetPieceType,
} from "./trainRoute";
import {
  TRAIN_SCENERY_BUILDINGS,
  TRAIN_SCENERY_DEPTH_GRAMMAR,
  TRAIN_SCENERY_LANDMARKS,
  TRAIN_SCENERY_VEGETATION,
  trainCoastSceneryBeatForChunk,
  trainForestMountainSceneryBeatForChunk,
  trainNightLifeForPlacement,
  trainSceneryPlacementsForChunk,
  trainTownIndustrialAssetScale,
  trainTownIndustrialSceneryBeatForChunk,
  type TrainCoastSceneryBeat,
  type TrainForestMountainSceneryBeat,
  type TrainNightLifePlan,
  type TrainSceneryAsset,
  type TrainSceneryPlacement,
  type TrainTownIndustrialFixtureKind,
  type TrainTownIndustrialSceneryBeat,
} from "./trainScenery";
import {
  generateTrainNightSkyCatalogue,
  type TrainStar,
} from "./trainStars";
import {
  generateTrainDaySkyCatalogue,
  type TrainDaySkyAnchor,
} from "./trainSky";
import {
  SCENE_MODES,
  clockSceneMode,
  nextClockSceneModeBoundary,
  nextSceneMode,
  type SceneMode,
} from "./sceneTime";
import {
  trainSkyAnchorPositionPx,
  trainWheelRotationDegrees,
  TRAIN_SKY_SUN_SPEED_RATIO,
  TRAIN_SKY_WISP_SPEED_RATIO,
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
import {
  loadTrainJourneySnapshot,
  TRAIN_JOURNEY_SNAPSHOT_VERSION,
  trainJourneyCheckpoint,
  trainJourneyPersistenceDue,
  trainJourneyStorage,
  writeTrainJourneySnapshot,
  type TrainJourneySnapshot,
} from "./trainJourneySnapshot";
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
const TRAIN_WORLD_POSITION_PARAM = "train-route-position";
const TRAIN_WORLD_REDUCED_MOTION_PARAM = "train-reduced-motion";
const TRAIN_WORLD_SET_PIECE_FOCUS_PARAM = "train-set-piece-focus";
const TRAIN_WORLD_SET_PIECE_OCCURRENCE_PARAM =
  "train-set-piece-occurrence";
const TRAIN_WORLD_MAX_DEVELOPMENT_POSITION = 1_000_000;
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

export interface TrainTimePalette {
  skyTop: string;
  skyBottom: string;
  haze: string;
  horizonLight: string;
  cloudLight: string;
  cloudShadow: string;
  silhouette: string;
  farSurface: string;
  midSurface: string;
  nearSurface: string;
  water: string;
  forestSoil: string;
  mountainRock: string;
  forestLife: string;
  mountainLife: string;
  townLife: string;
  coastLife: string;
  industrialLife: string;
  foregroundContrast: string;
  controlSurface: string;
  emissive: string;
}

interface TrainSceneryTimeGrade {
  saturation: number;
  brightness: number;
  warmth: number;
}

export const TRAIN_TIME_PALETTES: Readonly<Record<SceneMode, TrainTimePalette>> = {
  day: {
    skyTop: "#439fd2",
    skyBottom: "#c5edf4",
    haze: "rgba(194, 229, 239, 0.38)",
    horizonLight: "rgba(255, 238, 183, 0)",
    cloudLight: "#f6fbff",
    cloudShadow: "#78a7c0",
    silhouette: "#53767b",
    farSurface: "#426e64",
    midSurface: "#315c51",
    nearSurface: "#183f3b",
    water: "#4c9db5",
    forestSoil: "#4d704d",
    mountainRock: "#777887",
    forestLife: "#dff79b",
    mountainLife: "#ffd08a",
    townLife: "#ffd59a",
    coastLife: "#e8f4ff",
    industrialLife: "#ff7868",
    foregroundContrast: "#10243a",
    controlSurface: "#f4fbff",
    emissive: "#fff2ad",
  },
  sunset: {
    skyTop: "#465b82",
    skyBottom: "#efa16f",
    haze: "rgba(230, 174, 139, 0.32)",
    horizonLight: "rgba(255, 174, 101, 0.58)",
    cloudLight: "#ffd9a8",
    cloudShadow: "#7b6680",
    silhouette: "#51536b",
    farSurface: "#5f6071",
    midSurface: "#484d5f",
    nearSurface: "#293343",
    water: "#8a6f80",
    forestSoil: "#4d5d48",
    mountainRock: "#706c79",
    forestLife: "#dff29a",
    mountainLife: "#ffc77c",
    townLife: "#ffd28b",
    coastLife: "#ffe4ac",
    industrialLife: "#ff745f",
    foregroundContrast: "#fff6df",
    controlSurface: "#30364d",
    emissive: "#ffd889",
  },
  night: {
    skyTop: "#071326",
    skyBottom: "#142d49",
    haze: "rgba(74, 109, 145, 0.22)",
    horizonLight: "rgba(66, 94, 134, 0.12)",
    cloudLight: "#66809f",
    cloudShadow: "#1b304b",
    silhouette: "#162f49",
    farSurface: "#193c57",
    midSurface: "#14344b",
    nearSurface: "#0b2638",
    water: "#164d69",
    forestSoil: "#284337",
    mountainRock: "#484e62",
    forestLife: "#d8f58c",
    mountainLife: "#ffc979",
    townLife: "#ffd49a",
    coastLife: "#e7f3ff",
    industrialLife: "#ff715f",
    foregroundContrast: "#eaf6ff",
    controlSurface: "#07111f",
    emissive: "#ffe596",
  },
};

export const TRAIN_SCENERY_DEPTH_PROFILES = TRAIN_SCENERY_DEPTH_GRAMMAR;

export const TRAIN_SCENERY_TIME_GRADES: Readonly<
  Record<SceneMode, TrainSceneryTimeGrade>
> = {
  day: { saturation: 1, brightness: 1.06, warmth: 0 },
  sunset: { saturation: 0.98, brightness: 0.96, warmth: 0 },
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

export function trainPaletteLuminanceOrder(mode: SceneMode) {
  const palette = TRAIN_TIME_PALETTES[mode];
  return {
    skyTop: colorLuminance(palette.skyTop),
    skyBottom: colorLuminance(palette.skyBottom),
    farSurface: colorLuminance(palette.farSurface),
    midSurface: colorLuminance(palette.midSurface),
    nearSurface: colorLuminance(palette.nearSurface),
  };
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

export function trainWorldRoutePosition(search: string): number {
  if (!import.meta.env.DEV) return 0;
  const requested = Number.parseFloat(
    new URLSearchParams(search).get(TRAIN_WORLD_POSITION_PARAM) ?? "",
  );
  if (!Number.isFinite(requested) || requested < 0) return 0;
  return Math.min(TRAIN_WORLD_MAX_DEVELOPMENT_POSITION, requested);
}

export function trainWorldReducedMotionForced(search: string): boolean {
  return (
    import.meta.env.DEV &&
    new URLSearchParams(search).get(TRAIN_WORLD_REDUCED_MOTION_PARAM) === "1"
  );
}

const TRAIN_WORLD_FOCUS_TYPES = new Set<TrainSetPieceType>([
  "bridge",
  "tunnel",
  "coast-reveal",
  "town-edge",
  "station",
]);

export function trainWorldSetPieceFocus(
  search: string,
  seed: string,
  viewportWidth: number,
): TrainSetPieceFocus | null {
  if (!import.meta.env.DEV) return null;
  const parameters = new URLSearchParams(search);
  const requestedType = parameters
    .get(TRAIN_WORLD_SET_PIECE_FOCUS_PARAM)
    ?.trim() as TrainSetPieceType | undefined;
  if (!requestedType || !TRAIN_WORLD_FOCUS_TYPES.has(requestedType)) {
    return null;
  }
  const requestedOccurrence = Number.parseInt(
    parameters.get(TRAIN_WORLD_SET_PIECE_OCCURRENCE_PARAM) ?? "0",
    10,
  );
  const occurrence =
    Number.isInteger(requestedOccurrence) && requestedOccurrence >= 0
      ? Math.min(99, requestedOccurrence)
      : 0;
  return trainSetPieceFocusForOccurrence(
    seed,
    requestedType,
    viewportWidth,
    occurrence,
  );
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

type TrainTerrainBaseStyle = CSSProperties & {
  "--train-terrain-point-0": string;
  "--train-terrain-point-1": string;
  "--train-terrain-point-2": string;
  "--train-terrain-point-3": string;
  "--train-terrain-point-4": string;
  "--train-terrain-point-5": string;
  "--train-terrain-point-6": string;
  "--train-terrain-point-7": string;
};

type TrainSetPieceStyle = CSSProperties & {
  "--train-set-piece-phase": string;
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
  "--train-scenery-ground-height"?: string;
  "--train-scenery-foundation-width"?: string;
  "--train-cloud-drift-start"?: string;
  "--train-cloud-drift-end"?: string;
  "--train-cloud-drift-duration"?: string;
  "--train-cloud-drift-delay"?: string;
};

type TrainNightLifeStyle = CSSProperties & {
  "--train-night-life-intensity": number;
  "--train-scenery-scale": number;
  "--train-scenery-ground-height"?: string;
};

type TrainBuiltEnvironmentFixtureStyle = CSSProperties & {
  "--train-built-fixture-scale": number;
  "--train-built-fixture-ground-height": string;
  "--train-built-fixture-width"?: string;
  "--train-built-fixture-height"?: string;
  "--train-built-fixture-foundation-width"?: string;
};

type TrainCoastFixtureStyle = CSSProperties & {
  "--train-coast-fixture-scale": number;
  "--train-coast-fixture-ground-height": string;
  "--train-coast-waterline-height": string;
};

type TrainPaletteStyle = CSSProperties & {
  "--train-palette-sky-top": string;
  "--train-palette-sky-bottom": string;
  "--train-palette-haze": string;
  "--train-palette-horizon-light": string;
  "--train-palette-cloud-light": string;
  "--train-palette-cloud-shadow": string;
  "--train-palette-silhouette": string;
  "--train-palette-far-surface": string;
  "--train-palette-mid-surface": string;
  "--train-palette-near-surface": string;
  "--train-palette-water": string;
  "--train-palette-forest-soil": string;
  "--train-palette-mountain-rock": string;
  "--train-palette-life-forest": string;
  "--train-palette-life-mountain": string;
  "--train-palette-life-town": string;
  "--train-palette-life-coast": string;
  "--train-palette-life-industrial": string;
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

const TRAIN_STATION_SEGMENT_COMPOSITIONS = [
  {
    bay: "entrance",
    structure: "gatehouse",
    massRole: "entry-house",
    hasBuilding: true,
    hasCanopy: true,
    canopyRole: "entry-awning",
    opening: "entry-vista",
    door: "single",
    entrance: "side-entrance",
    hasWindows: false,
    windowLight: null,
    lampSlots: [],
    supportSlots: ["trailing"],
    serviceElements: ["wayfinding"],
    signalAspect: "approach",
  },
  {
    bay: "west-waiting",
    structure: "waiting-bay",
    massRole: "open-waiting",
    hasBuilding: false,
    hasCanopy: true,
    canopyRole: "waiting-shelter",
    opening: "waiting-vista",
    door: null,
    entrance: null,
    hasWindows: false,
    windowLight: null,
    lampSlots: ["leading"],
    supportSlots: ["leading", "trailing"],
    serviceElements: ["bench"],
    signalAspect: null,
  },
  {
    bay: "ticket-hall",
    structure: "station-house",
    massRole: "ticket-house",
    hasBuilding: true,
    hasCanopy: true,
    canopyRole: "ticket-awning",
    opening: "ticket-vista",
    door: "double",
    entrance: "main-entrance",
    hasWindows: true,
    windowLight: "sunset-night",
    lampSlots: ["trailing"],
    supportSlots: ["leading", "trailing"],
    serviceElements: ["timetable"],
    signalAspect: null,
  },
  {
    bay: "garden-platform",
    structure: "garden-bay",
    massRole: "open-garden",
    hasBuilding: false,
    hasCanopy: false,
    canopyRole: null,
    opening: "garden-vista",
    door: null,
    entrance: null,
    hasWindows: false,
    windowLight: null,
    lampSlots: ["center"],
    supportSlots: [],
    serviceElements: ["bench", "planter"],
    signalAspect: null,
  },
  {
    bay: "service-yard",
    structure: "service-shed",
    massRole: "service-house",
    hasBuilding: true,
    hasCanopy: false,
    canopyRole: null,
    opening: "service-vista",
    door: "freight",
    entrance: "service-entrance",
    hasWindows: true,
    windowLight: "night",
    lampSlots: [],
    supportSlots: [],
    serviceElements: ["baggage-cart", "parcel-stack"],
    signalAspect: null,
  },
  {
    bay: "departure",
    structure: "exit-platform",
    massRole: "open-departure",
    hasBuilding: false,
    hasCanopy: true,
    canopyRole: "departure-shelter",
    opening: "exit-vista",
    door: null,
    entrance: null,
    hasWindows: false,
    windowLight: null,
    lampSlots: ["leading"],
    supportSlots: ["leading", "trailing"],
    serviceElements: ["wayfinding"],
    signalAspect: "proceed",
  },
] as const;

function TrainNightLife({
  plan,
  placement,
  groundHeight,
  contourHeight,
  instanceId,
  waterOwner,
}: {
  plan: TrainNightLifePlan;
  placement: TrainSceneryPlacement;
  groundHeight?: number;
  contourHeight?: number;
  instanceId: string;
  waterOwner?: string;
}) {
  const { asset } = placement;
  const style: TrainNightLifeStyle = {
    left: `${placement.offsetPercent}%`,
    width: `${asset.width}px`,
    height: `${asset.height}px`,
    "--train-night-life-intensity": plan.intensity,
    "--train-scenery-scale": placement.scale,
    "--train-scenery-ground-height":
      groundHeight === undefined ? undefined : `${groundHeight}px`,
  };

  return (
    <span
      className={[
        "train-night-life",
        `train-night-life--${plan.kind}`,
        `train-night-life--variant-${plan.variant}`,
      ].join(" ")}
      data-emissive={plan.kind}
      data-emissive-owner={plan.ownerAssetId}
      data-emissive-owner-instance={instanceId}
      data-emissive-plane="owner-attached"
      data-emissive-ground-height={groundHeight?.toFixed(3)}
      data-emissive-contour-height={contourHeight?.toFixed(3)}
      data-emissive-ground-inset={placement.groundInsetPx.toFixed(3)}
      data-emissive-scale={placement.scale.toFixed(3)}
      data-night-life-region={plan.region}
      data-night-life-palette={plan.paletteToken}
      data-night-life-kind={plan.kind}
      data-night-life-variant={plan.variant}
      data-night-life-intensity={plan.intensity.toFixed(3)}
      data-night-life-plane="midground-behind-train"
      data-night-life-reflection={plan.pairedReflection ? "paired" : "none"}
      style={style}
    >
      {plan.kind === "forest-fireflies"
        ? plan.points.map((point, pointIndex) => (
            <span
              className="train-night-life__firefly"
              data-night-life-detail="firefly"
              style={{
                left: `${point.xPercent}%`,
                top: `${point.yPercent}%`,
                animationDelay: `${point.delayMs}ms`,
              }}
              key={pointIndex}
            />
          ))
        : null}
      {plan.kind === "mountain-lookout-glow" ? (
        <>
          <span
            className="train-night-life__lookout-window"
            data-night-life-detail="lookout-window"
          />
          <span
            className="train-night-life__camp-glow"
            data-night-life-detail="camp-glow"
          />
        </>
      ) : null}
      {plan.kind === "town-settlement-glow" ? (
        <>
          <span
            className="train-night-life__church-window train-night-life__church-window--tower"
            data-night-life-detail="occupied-window"
          />
          <span
            className="train-night-life__church-window train-night-life__church-window--nave"
            data-night-life-detail="occupied-window"
          />
        </>
      ) : null}
      {plan.kind === "coast-lighthouse-beacon" ? (
        <>
          <span
            className="train-night-life__lighthouse-lantern"
            data-night-life-detail="lighthouse-lantern"
          />
          <span
            className="train-night-life__lighthouse-beam"
            data-night-life-detail="lighthouse-beam"
          />
          <span
            className="train-coast-reflection-clip"
            data-reflection-clip="water-only"
            data-reflection-clip-owner={waterOwner}
            data-water-owner={waterOwner}
          >
            <span
              className="train-night-life__lighthouse-reflection"
              data-emissive="lighthouse-water-reflection"
              data-emissive-owner={plan.ownerAssetId}
              data-reflection-source-owner={instanceId}
              data-night-life-detail="paired-reflection"
            />
          </span>
        </>
      ) : null}
      {plan.kind === "industrial-beacons" ? (
        <>
          <span
            className="train-night-life__industrial-beacon"
            data-night-life-detail="industrial-beacon"
          />
          <span
            className="train-night-life__industrial-steam"
            data-night-life-detail="restrained-steam"
          />
        </>
      ) : null}
    </span>
  );
}

type TrainStarStyle = CSSProperties & {
  "--train-star-brightness": number;
  "--train-star-size": string;
};

type TrainMoonStyle = CSSProperties & {
  "--train-moon-size": string;
  "--train-moon-shadow-offset": string;
  "--train-moon-shadow-opacity": number;
  "--train-moon-shadow-scale": number;
};

type TrainCelestialBandStyle = CSSProperties & {
  "--train-celestial-band-height": string;
  "--train-celestial-band-opacity": number;
  "--train-celestial-band-rotation": string;
};

type TrainCelestialAccentStyle = CSSProperties & {
  "--train-celestial-accent-opacity": number;
};

type TrainDaySkyAnchorStyle = CSSProperties & {
  "--train-sky-anchor-opacity": number;
  "--train-sky-anchor-width": string;
  "--train-sky-anchor-height": string;
  "--train-sky-anchor-x": string;
  "--train-sky-anchor-day-y": string;
  "--train-sky-anchor-sunset-y": string;
};

type TrainTownEdgeAssetID =
  | "building-rowhouse"
  | "building-apartments"
  | "building-cottage"
  | "landmark-town-church";

interface TrainTownEdgeBuildingPlan {
  assetID: TrainTownEdgeAssetID;
  xPercent: number;
  scale: number;
  liftPx: number;
  material: "brick" | "stone" | "plaster";
}

type TrainTownEdgeBuildingStyle = CSSProperties & {
  "--train-town-edge-building-scale": number;
};

const TRAIN_TOWN_EDGE_VARIANT_PLANS = [
  [
    [
      { assetID: "building-cottage", xPercent: 76, scale: 0.64, liftPx: 1, material: "plaster" },
    ],
    [
      { assetID: "building-rowhouse", xPercent: 16, scale: 0.68, liftPx: 0, material: "brick" },
      { assetID: "landmark-town-church", xPercent: 52, scale: 0.6, liftPx: 0, material: "stone" },
      { assetID: "building-cottage", xPercent: 86, scale: 0.64, liftPx: 1, material: "plaster" },
    ],
    [
      { assetID: "building-rowhouse", xPercent: 9, scale: 0.67, liftPx: 0, material: "brick" },
      { assetID: "building-apartments", xPercent: 35, scale: 0.61, liftPx: 0, material: "stone" },
      { assetID: "building-cottage", xPercent: 62, scale: 0.63, liftPx: 1, material: "plaster" },
      { assetID: "building-rowhouse", xPercent: 88, scale: 0.66, liftPx: 0, material: "brick" },
    ],
  ],
  [
    [
      { assetID: "building-rowhouse", xPercent: 72, scale: 0.67, liftPx: 0, material: "brick" },
    ],
    [
      { assetID: "building-cottage", xPercent: 14, scale: 0.63, liftPx: 1, material: "plaster" },
      { assetID: "building-apartments", xPercent: 48, scale: 0.59, liftPx: 0, material: "stone" },
      { assetID: "building-rowhouse", xPercent: 84, scale: 0.69, liftPx: 0, material: "brick" },
    ],
    [
      { assetID: "building-cottage", xPercent: 8, scale: 0.63, liftPx: 1, material: "plaster" },
      { assetID: "building-rowhouse", xPercent: 33, scale: 0.68, liftPx: 0, material: "brick" },
      { assetID: "landmark-town-church", xPercent: 60, scale: 0.61, liftPx: 0, material: "stone" },
      { assetID: "building-apartments", xPercent: 87, scale: 0.6, liftPx: 0, material: "stone" },
    ],
  ],
] as const satisfies readonly (readonly (readonly TrainTownEdgeBuildingPlan[])[])[];

const TRAIN_TOWN_EDGE_ASSETS: readonly TrainSceneryAsset[] = [
  ...TRAIN_SCENERY_BUILDINGS,
  ...TRAIN_SCENERY_LANDMARKS,
];

function trainTownEdgeAsset(assetID: TrainTownEdgeAssetID): TrainSceneryAsset {
  const asset = TRAIN_TOWN_EDGE_ASSETS.find((candidate) => candidate.id === assetID);
  if (!asset) throw new Error(`Missing town-edge asset: ${assetID}`);
  return asset;
}

function trainPaletteStyle(mode: SceneMode): TrainPaletteStyle {
  const palette = TRAIN_TIME_PALETTES[mode];
  const grade = TRAIN_SCENERY_TIME_GRADES[mode];
  return {
    "--train-palette-sky-top": palette.skyTop,
    "--train-palette-sky-bottom": palette.skyBottom,
    "--train-palette-haze": palette.haze,
    "--train-palette-horizon-light": palette.horizonLight,
    "--train-palette-cloud-light": palette.cloudLight,
    "--train-palette-cloud-shadow": palette.cloudShadow,
    "--train-palette-silhouette": palette.silhouette,
    "--train-palette-far-surface": palette.farSurface,
    "--train-palette-mid-surface": palette.midSurface,
    "--train-palette-near-surface": palette.nearSurface,
    "--train-palette-water": palette.water,
    "--train-palette-forest-soil": palette.forestSoil,
    "--train-palette-mountain-rock": palette.mountainRock,
    "--train-palette-life-forest": palette.forestLife,
    "--train-palette-life-mountain": palette.mountainLife,
    "--train-palette-life-town": palette.townLife,
    "--train-palette-life-coast": palette.coastLife,
    "--train-palette-life-industrial": palette.industrialLife,
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

function TrainTownEdgeComposition({
  segment,
  layer,
}: {
  segment: NonNullable<RouteChunk["setPiece"]>;
  layer: TrainParallaxLayerName;
}) {
  const plan =
    TRAIN_TOWN_EDGE_VARIANT_PLANS[segment.visualVariant][segment.segmentOffset];
  if (!plan) return null;
  const density = ["open-edge", "gathering", "settled-block"][
    segment.segmentOffset
  ];

  return (
    <span
      className={[
        "train-town-edge-transition",
        `train-town-edge-transition--${layer}`,
      ].join(" ")}
      data-town-edge-transition="open-land-to-settlement"
      data-town-edge-variant={segment.visualVariant}
      data-town-edge-role={segment.role}
      data-town-edge-segment={segment.segmentOffset}
      data-town-edge-layer={layer}
      data-town-edge-density={density}
      data-town-edge-road-grammar={
        segment.visualVariant === 0 ? "market-road" : "garden-lane"
      }
      data-town-edge-clearance="foreground-reserved"
      aria-hidden="true"
    >
      {layer === "midground" ? (
        <>
          <span
            className="train-town-edge-road"
            data-town-edge-geometry="road"
            data-town-edge-surface="opaque"
          />
          <span
            className="train-town-edge-yard"
            data-town-edge-geometry="yard"
            data-town-edge-surface="opaque"
          >
            <i data-town-edge-yard-detail="gate" />
            <i data-town-edge-yard-detail="tree" />
          </span>
          <span
            className="train-town-edge-composition"
            data-town-edge-composition="density-gradient-settlement"
          >
            {plan.map((building, slot) => {
              const asset = trainTownEdgeAsset(building.assetID);
              const globalSlot =
                TRAIN_TOWN_EDGE_VARIANT_PLANS[segment.visualVariant]
                  .slice(0, segment.segmentOffset)
                  .reduce((total, buildings) => total + buildings.length, 0) +
                slot;
              const style: TrainTownEdgeBuildingStyle = {
                left: `${building.xPercent}%`,
                bottom: `${building.liftPx}px`,
                "--train-town-edge-building-scale": building.scale,
              };
              return (
                <span
                  className="train-town-edge-building-shell"
                  data-town-edge-slot={globalSlot}
                  data-town-edge-continuity={`${segment.visualVariant}:${globalSlot}`}
                  data-town-edge-material={building.material}
                  style={style}
                  key={`${building.assetID}-${globalSlot}`}
                >
                  <img
                    className="train-town-edge-building"
                    src={asset.src}
                    alt=""
                    aria-hidden="true"
                    draggable={false}
                    width={asset.width}
                    height={asset.height}
                    data-town-edge-building={asset.id}
                    data-town-edge-solid="opaque"
                  />
                  {asset.emissive ? (
                    <img
                      className="train-emissive-overlay train-town-edge-building-emissive"
                      src={asset.emissive.src}
                      alt=""
                      aria-hidden="true"
                      draggable={false}
                      width={asset.emissive.width}
                      height={asset.emissive.height}
                      data-emissive="town-edge-windows"
                      data-emissive-owner={asset.id}
                      data-emissive-region="town"
                      data-town-edge-window-alignment={`${asset.width}x${asset.height}`}
                      onError={(event) => {
                        event.currentTarget.hidden = true;
                      }}
                    />
                  ) : null}
                </span>
              );
            })}
          </span>
        </>
      ) : (
        <span
          className="train-town-edge-foreground"
          data-town-edge-geometry="foreground-clearing"
          data-town-edge-surface="opaque"
        >
          <i data-town-edge-foreground-detail="verge" />
          <i data-town-edge-foreground-detail="fence" />
        </span>
      )}
    </span>
  );
}

function TrainCoastRevealComposition({
  segment,
  layer,
}: {
  segment: NonNullable<RouteChunk["setPiece"]>;
  layer: TrainParallaxLayerName;
}) {
  const waterCoverage = (
    segment.visualVariant === 0 ? [58, 100, 100, 62] : [64, 100, 100, 70]
  )[segment.segmentOffset]!;
  return (
    <span
      className={[
        "train-coast-reveal-composition",
        `train-coast-reveal-composition--${layer}`,
      ].join(" ")}
      data-coast-reveal-composition={
        segment.visualVariant === 0 ? "open-bay" : "harbour-mouth"
      }
      data-coast-reveal-layer={layer}
      data-coast-reveal-role={segment.role}
      data-coast-reveal-segment={segment.segmentOffset}
      data-coast-reveal-water-coverage={waterCoverage}
      data-coast-reveal-clearance="foreground-reserved"
      data-coast-reveal-layer-role={
        layer === "far"
          ? "water-horizon"
          : layer === "midground"
            ? "shoreline-frame"
            : "track-foreground"
      }
      data-coast-reveal-single-owner="true"
      aria-hidden="true"
    >
      {layer === "far" ? (
        <>
          <span
            className="train-coast-reveal-water"
            data-coast-reveal-geometry="broad-water"
            data-coast-reveal-geometry-owner={`${segment.id}:far:${segment.segmentOffset}`}
            data-coast-contact-medium="water"
            data-water-owner={`${segment.id}:far`}
            data-water-surface="opaque"
          >
            <i data-coast-water-depth="horizon" />
            <i data-coast-water-depth="middle" />
            <i data-coast-water-depth="near" />
          </span>
          <span
            className="train-coast-reveal-horizon"
            data-coast-reveal-geometry="water-horizon"
            data-coast-reveal-geometry-owner={`${segment.id}:far:${segment.segmentOffset}`}
          />
        </>
      ) : layer === "midground" ? (
        <span
          className="train-coast-reveal-shore"
          data-coast-reveal-geometry="shoreline-frame"
          data-coast-reveal-geometry-owner={`${segment.id}:midground:${segment.segmentOffset}`}
          data-coast-reveal-surface="opaque"
          data-coast-contact-medium="dry-land"
        >
          <i data-coast-reveal-shore-detail="rock-shelf" />
          <i data-coast-reveal-shore-detail="beach" />
        </span>
      ) : (
        <span
          className="train-coast-reveal-foreground"
          data-coast-reveal-geometry="foreground-opening"
          data-coast-reveal-geometry-owner={`${segment.id}:near:${segment.segmentOffset}`}
          data-coast-reveal-surface="opaque"
          data-coast-contact-medium="dry-land"
        >
          <i data-coast-reveal-foreground-detail="headland" />
          <i data-coast-reveal-foreground-detail="coastal-grass" />
        </span>
      )}
    </span>
  );
}

const TRAIN_TRANSITION_TYPES = new Set<TrainSetPieceType>([
  "coast-reveal",
  "town-edge",
]);

const TRAIN_TRAVERSAL_TYPES = new Set<TrainSetPieceType>([
  "bridge",
  "tunnel",
]);

function TrainTraversalComposition({
  segment,
  layer,
}: {
  segment: NonNullable<RouteChunk["setPiece"]>;
  layer: TrainParallaxLayerName;
}) {
  if (!TRAIN_TRAVERSAL_TYPES.has(segment.type)) return null;

  return (
    <span
      className={[
        "train-traversal-composition",
        `train-traversal-composition--${segment.type}`,
        `train-traversal-composition--${layer}`,
      ].join(" ")}
      data-traversal-composition={segment.type}
      data-traversal-layer={layer}
      data-traversal-role={segment.role}
      data-traversal-variant={segment.visualVariant}
      data-traversal-segment={segment.segmentOffset}
      data-traversal-track-contact="17"
      data-traversal-geometry-owner={`${segment.id}:${layer}:${segment.segmentOffset}`}
      data-tunnel-state={segment.type === "tunnel" ? segment.role : undefined}
      data-tunnel-opening-count={
        segment.type === "tunnel"
          ? layer === "midground"
            ? "1"
            : "0"
          : undefined
      }
      data-tunnel-layer-responsibility={
        segment.type === "tunnel"
          ? layer === "midground"
            ? "rock-portal-bore"
            : "trackside-contact"
          : undefined
      }
      data-bridge-state={segment.type === "bridge" ? segment.role : undefined}
      data-bridge-layer-responsibility={
        segment.type === "bridge"
          ? layer === "midground"
            ? "crossing-deck-structure"
            : "trackside-contact"
          : undefined
      }
      data-bridge-crossing-subject={
        segment.type === "bridge"
          ? layer === "midground"
            ? segment.visualVariant === 0
              ? "river"
              : "gorge"
            : "none"
          : undefined
      }
      data-bridge-single-owner={
        segment.type === "bridge" ? "true" : undefined
      }
      aria-hidden="true"
    >
      {segment.type === "bridge" ? (
        <>
          {layer === "midground" ? (
            <>
              <span
                className="train-bridge-crossing-void"
                data-bridge-geometry={`${segment.role}-crossing-void`}
                data-bridge-geometry-owner={`${segment.id}:midground:crossing`}
                data-bridge-crossing-medium={
                  segment.visualVariant === 0 ? "river" : "gorge"
                }
                data-bridge-solid-surface="opaque"
              >
                <i data-bridge-crossing-detail="far-bank" />
                <i data-bridge-crossing-detail="crossing-floor" />
                <i data-bridge-crossing-detail="near-bank" />
              </span>
              {segment.role === "body" ? null : (
                <span
                  className="train-bridge-approach"
                  data-bridge-geometry={`${segment.role}-approach`}
                  data-bridge-geometry-owner={`${segment.id}:midground:${segment.role}-approach`}
                  data-bridge-ground-owner="midground"
                  data-bridge-solid-surface="opaque"
                />
              )}
              <span
                className="train-bridge-span"
                data-bridge-geometry={`${segment.role}-${
                  segment.visualVariant === 0
                    ? "pony-truss"
                    : "stone-parapet"
                }`}
                data-bridge-geometry-owner={`${segment.id}:midground:structure`}
              >
                <i className="train-bridge-upper-chord" />
                <i className="train-bridge-lattice" />
                <i className="train-bridge-deck-brace" />
              </span>
              <span
                className="train-bridge-deck"
                data-bridge-geometry="track-deck"
                data-bridge-geometry-owner={`${segment.id}:midground:deck`}
                data-bridge-deck-continuity={`${segment.role}:${segment.segmentOffset}`}
                data-track-contact="17"
              />
              <span
                className="train-bridge-supports"
                data-bridge-geometry="supports-below-deck"
                data-bridge-geometry-owner={`${segment.id}:midground:supports`}
              >
                <i data-bridge-support="left" />
                <i data-bridge-support="centre" />
                <i data-bridge-support="right" />
              </span>
            </>
          ) : (
            <span
              className="train-bridge-track-edge"
              data-bridge-geometry={`${segment.role}-track-edge`}
              data-bridge-geometry-owner={`${segment.id}:near:track-edge`}
              data-bridge-ground-owner="near"
              data-track-contact="17"
            />
          )}
        </>
      ) : (
        <>
          {layer === "midground" ? (
            <>
              <span
                className="train-tunnel-mountain-mass"
                data-tunnel-geometry="enclosing-mountain"
                data-tunnel-geometry-owner={`${segment.id}:midground:rock`}
                data-tunnel-solid-surface="opaque"
              >
                <i data-tunnel-rock-detail="upper-facet" />
                <i data-tunnel-rock-detail="lower-strata" />
              </span>
              <span
                className="train-tunnel-opening"
                data-tunnel-geometry={`${segment.role}-bore`}
                data-tunnel-opening-owner={`${segment.id}:midground:${segment.segmentOffset}`}
                data-tunnel-bore-continuity="rail-passage"
                data-track-contact="17"
              />
              <span
                className="train-tunnel-portal"
                data-tunnel-geometry={
                  segment.role === "body"
                    ? "bore-lining"
                    : `${segment.role}-portal-frame`
                }
                data-tunnel-portal-visible={
                  segment.role === "body" ? "passage" : "portal"
                }
                data-tunnel-portal-silhouette={
                  segment.visualVariant === 0 ? "round-arch" : "stepped-arch"
                }
                data-track-contact="17"
              >
                <i data-tunnel-lining-rib="left" />
                <i data-tunnel-lining-rib="crown" />
                <i data-tunnel-lining-rib="right" />
              </span>
            </>
          ) : (
            <span
              className="train-tunnel-track-edge"
              data-tunnel-geometry={`${segment.role}-track-edge`}
              data-tunnel-geometry-owner={`${segment.id}:near:track-edge`}
              data-tunnel-ground-owner="near"
              data-track-contact="17"
            >
              <i />
              <i />
            </span>
          )}
        </>
      )}
    </span>
  );
}

type TrainSolidTerrainLayer = Exclude<TrainParallaxLayerName, "sky">;

export interface TrainTerrainContourPoint {
  xPercent: number;
  heightPx: number;
}

export interface TrainTerrainContour {
  layer: TrainSolidTerrainLayer;
  region: RouteChunk["region"];
  variant: number;
  material: TrainTerrainMaterial;
  transitionMaterial: TrainTerrainMaterial | null;
  points: readonly TrainTerrainContourPoint[];
  seamLeftHeightPx: number;
  seamRightHeightPx: number;
}

export type TrainTerrainMaterial =
  | "forest-soil"
  | "mountain-rock"
  | "town-ground"
  | "coast-shore"
  | "industrial-fill";

export const TRAIN_TERRAIN_REGION_MATERIALS = {
  forest: "forest-soil",
  mountain: "mountain-rock",
  town: "town-ground",
  coast: "coast-shore",
  industrial: "industrial-fill",
} as const satisfies Record<RouteChunk["region"], TrainTerrainMaterial>;

export const TRAIN_TERRAIN_LAYER_ENVELOPES = {
  "ultra-far": {
    baseHeightPx: 108,
    minimumHeightPx: 76,
    maximumHeightPx: 164,
    minimumVariationPx: 14,
    routeNoiseScale: 0.62,
    reliefScale: 1,
  },
  far: {
    baseHeightPx: 82,
    minimumHeightPx: 56,
    maximumHeightPx: 132,
    minimumVariationPx: 12,
    routeNoiseScale: 0.5,
    reliefScale: 0.82,
  },
  midground: {
    baseHeightPx: 51,
    minimumHeightPx: 36,
    maximumHeightPx: 102,
    minimumVariationPx: 8,
    routeNoiseScale: 0.34,
    reliefScale: 0.62,
  },
  near: {
    baseHeightPx: 22,
    minimumHeightPx: 19,
    maximumHeightPx: 36,
    minimumVariationPx: 3,
    routeNoiseScale: 0.06,
    reliefScale: 0.22,
  },
} as const satisfies Record<
  TrainSolidTerrainLayer,
  {
    baseHeightPx: number;
    minimumHeightPx: number;
    maximumHeightPx: number;
    minimumVariationPx: number;
    routeNoiseScale: number;
    reliefScale: number;
  }
>;

const TRAIN_TERRAIN_CONTOUR_X = [0, 11, 26, 43, 61, 77, 91, 100] as const;

const TRAIN_TERRAIN_REGION_HEIGHT_OFFSETS = {
  forest: { "ultra-far": 5, far: 6, midground: 6, near: 2 },
  mountain: { "ultra-far": 24, far: 21, midground: 17, near: 3 },
  town: { "ultra-far": -5, far: -3, midground: 1, near: 2 },
  coast: { "ultra-far": -17, far: -14, midground: -9, near: 0 },
  industrial: { "ultra-far": 0, far: 3, midground: 5, near: 3 },
} as const satisfies Record<
  RouteChunk["region"],
  Record<TrainSolidTerrainLayer, number>
>;

const TRAIN_TERRAIN_REGION_RELIEF = {
  forest: [17, 6, 22, 10, 19, 8],
  mountain: [31, 16, 44, 23, 37, 12],
  town: [9, 3, 14, 6, 11, 4],
  coast: [-5, 8, -9, 4, -2, 10],
  industrial: [13, 5, 18, 8, 15, 3],
} as const satisfies Record<
  RouteChunk["region"],
  readonly [number, number, number, number, number, number]
>;

function clampTrainTerrainHeight(
  heightPx: number,
  layer: TrainSolidTerrainLayer,
): number {
  const envelope = TRAIN_TERRAIN_LAYER_ENVELOPES[layer];
  return Math.max(
    envelope.minimumHeightPx,
    Math.min(envelope.maximumHeightPx, heightPx),
  );
}

function trainTerrainAnchorHeight(
  chunk: RouteChunk,
  layer: TrainSolidTerrainLayer,
): number {
  const envelope = TRAIN_TERRAIN_LAYER_ENVELOPES[layer];
  const regionOffset = TRAIN_TERRAIN_REGION_HEIGHT_OFFSETS[chunk.region][layer];
  const routeNoise = (chunk.terrainHeight - 46) * envelope.routeNoiseScale;
  const nearStability =
    layer === "near" ? ((chunk.variant + chunk.regionIndex) % 3) - 1 : 0;
  return clampTrainTerrainHeight(
    Math.round(envelope.baseHeightPx + regionOffset + routeNoise + nearStability),
    layer,
  );
}

export function trainTerrainContourForChunk(
  chunk: RouteChunk,
  layer: TrainSolidTerrainLayer,
): TrainTerrainContour {
  // Route indices increase toward the left because the locomotive faces left.
  // Consequently the physical right neighbour is index - 1. Sharing that
  // neighbour's anchor as this contour's right endpoint gives both chunks the
  // exact same seam height without storing cross-chunk mutable state.
  const rightChunk = generateRouteChunk(
    chunk.routeSeed,
    chunk.index - 1,
    chunk.seedVersion,
  );
  const leftHeight = trainTerrainAnchorHeight(chunk, layer);
  const rightHeight = trainTerrainAnchorHeight(rightChunk, layer);
  const anchorHeight = trainTerrainAnchorHeight(chunk, layer);
  const relief = TRAIN_TERRAIN_REGION_RELIEF[chunk.region];
  const reliefScale = TRAIN_TERRAIN_LAYER_ENVELOPES[layer].reliefScale;
  const ridgeUnit = (chunk.ridgeHeight - 48) / 32;
  const points = TRAIN_TERRAIN_CONTOUR_X.map((xPercent, pointIndex) => {
    if (pointIndex === 0) {
      return { xPercent, heightPx: leftHeight };
    }
    if (pointIndex === TRAIN_TERRAIN_CONTOUR_X.length - 1) {
      return { xPercent, heightPx: rightHeight };
    }
    const progress = xPercent / 100;
    const seamLine = leftHeight + (rightHeight - leftHeight) * progress;
    const anchorBias =
      (anchorHeight - seamLine) * Math.sin(Math.PI * progress) * 0.7;
    const reliefIndex = (pointIndex - 1 + chunk.variant) % relief.length;
    const shapedRelief =
      (relief[reliefIndex]! +
        ridgeUnit * (pointIndex % 2 === 0 ? 3.2 : -1.4)) *
      reliefScale;
    return {
      xPercent,
      heightPx: clampTrainTerrainHeight(
        Math.round((seamLine + anchorBias + shapedRelief) * 1000) / 1000,
        layer,
      ),
    };
  });
  const envelope = TRAIN_TERRAIN_LAYER_ENVELOPES[layer];
  const contourMinimum = Math.min(...points.map((point) => point.heightPx));
  const contourMaximum = Math.max(...points.map((point) => point.heightPx));
  if (contourMaximum - contourMinimum < envelope.minimumVariationPx) {
    const peakIndex = Math.max(
      1,
      points.findIndex(
        (point, pointIndex) =>
          pointIndex > 0 &&
          pointIndex < points.length - 1 &&
          point.heightPx > contourMinimum,
      ),
    );
    points[peakIndex] = {
      ...points[peakIndex]!,
      heightPx: Math.min(
        envelope.maximumHeightPx,
        contourMinimum + envelope.minimumVariationPx,
      ),
    };
  }

  return {
    layer,
    region: chunk.region,
    variant: chunk.variant,
    material: TRAIN_TERRAIN_REGION_MATERIALS[chunk.region],
    transitionMaterial:
      rightChunk.region === chunk.region
        ? null
        : TRAIN_TERRAIN_REGION_MATERIALS[rightChunk.region],
    points,
    seamLeftHeightPx: leftHeight,
    seamRightHeightPx: rightHeight,
  };
}

export function trainTerrainHeightAtPercent(
  contour: TrainTerrainContour,
  requestedPercent: number,
): number {
  const percent = Math.max(0, Math.min(100, requestedPercent));
  for (let pointIndex = 1; pointIndex < contour.points.length; pointIndex++) {
    const right = contour.points[pointIndex]!;
    if (percent > right.xPercent) continue;
    const left = contour.points[pointIndex - 1]!;
    const progress =
      (percent - left.xPercent) / (right.xPercent - left.xPercent);
    return (
      Math.round(
        (left.heightPx + (right.heightPx - left.heightPx) * progress) * 1000,
      ) / 1000
    );
  }
  return contour.points.at(-1)!.heightPx;
}

const TRAIN_BUILT_ENVIRONMENT_FIXTURE_POSITIONS = [
  [20, 72],
  [28, 78],
  [18, 64],
] as const;

const TRAIN_BUILT_ENVIRONMENT_EMISSIVE_FIXTURES =
  new Set<TrainTownIndustrialFixtureKind>([
    "civic-clock",
    "vent-stack",
    "furnace-stack",
    "gantry-crane",
  ]);

const TRAIN_BUILT_ENVIRONMENT_RASTER_FIXTURES = {
  "townhouse-block": [
    "building-rowhouse",
    "building-apartments",
    "building-cottage",
  ],
  "shop-awning": ["building-rowhouse", "building-apartments"],
  "industrial-shed": ["building-workshop", "building-warehouse"],
  "gantry-crane": ["landmark-industrial-gantry"],
} as const satisfies Partial<
  Record<TrainTownIndustrialFixtureKind, readonly string[]>
>;

const TRAIN_BUILT_ENVIRONMENT_FIXTURE_COLLISION_WIDTHS = {
  fence: 72,
  "street-tree": 41,
  "civic-clock": 20,
  "yard-gate": 72,
  "utility-pole": 41,
  "vent-stack": 17,
  "storage-tank": 57,
  "furnace-stack": 21,
  "gantry-crane": 88,
  "service-pipe": 76,
} as const satisfies Partial<Record<TrainTownIndustrialFixtureKind, number>>;

function trainBuiltEnvironmentFixtureAsset(
  fixture: TrainTownIndustrialFixtureKind,
  beat: TrainTownIndustrialSceneryBeat,
  chunkIndex: number,
  fixtureIndex: number,
): TrainSceneryAsset | null {
  const pool =
    TRAIN_BUILT_ENVIRONMENT_RASTER_FIXTURES[
      fixture as keyof typeof TRAIN_BUILT_ENVIRONMENT_RASTER_FIXTURES
    ];
  if (!pool) return null;
  const poolIndex =
    ((chunkIndex + beat.templateVariant + fixtureIndex) % pool.length +
      pool.length) %
    pool.length;
  const assetID = pool[poolIndex]!;
  return (
    [...TRAIN_SCENERY_BUILDINGS, ...TRAIN_SCENERY_LANDMARKS].find(
      (candidate) => candidate.id === assetID,
    ) ??
    null
  );
}

function TrainBuiltEnvironmentFixtures({
  beat,
  contour,
  chunkIndex,
}: {
  beat: TrainTownIndustrialSceneryBeat;
  contour: TrainTerrainContour;
  chunkIndex: number;
}) {
  const positions =
    TRAIN_BUILT_ENVIRONMENT_FIXTURE_POSITIONS[beat.templateVariant] ??
    TRAIN_BUILT_ENVIRONMENT_FIXTURE_POSITIONS[0];
  return (
    <span
      className={[
        "train-built-environment-fixtures",
        `train-built-environment-fixtures--${beat.region}`,
      ].join(" ")}
      data-built-environment={beat.region}
      data-built-composition-family={beat.compositionFamily}
      data-built-scale-family={beat.scaleFamily}
      data-built-density={beat.densityClass}
      aria-hidden="true"
    >
      {beat.fixtures.map((fixture, fixtureIndex) => {
        const xPercent = positions[fixtureIndex] ?? 50;
        const contourHeight = trainTerrainHeightAtPercent(contour, xPercent);
        const rasterAsset = trainBuiltEnvironmentFixtureAsset(
          fixture,
          beat,
          chunkIndex,
          fixtureIndex,
        );
        const fixtureScale = rasterAsset
          ? trainTownIndustrialAssetScale(
              rasterAsset,
              beat.templateVariant + fixtureIndex,
              beat,
            ) * TRAIN_SCENERY_DEPTH_GRAMMAR.midground.scaleMultiplier
          : 0.86 + ((beat.templateVariant + fixtureIndex) % 3) * 0.08;
        const groundInset = (rasterAsset?.groundInsetPx ?? 0) * fixtureScale;
        const groundHeight = contourHeight - groundInset;
        const collisionWidth =
          (rasterAsset?.collisionWidth ??
            TRAIN_BUILT_ENVIRONMENT_FIXTURE_COLLISION_WIDTHS[
              fixture as keyof typeof TRAIN_BUILT_ENVIRONMENT_FIXTURE_COLLISION_WIDTHS
            ] ??
            40) * fixtureScale;
        const owner = `${beat.region}:${chunkIndex}:${fixtureIndex}:${fixture}`;
        const style: TrainBuiltEnvironmentFixtureStyle = {
          left: `${xPercent}%`,
          "--train-built-fixture-scale": fixtureScale,
          "--train-built-fixture-ground-height": `${groundHeight}px`,
          "--train-built-fixture-width": rasterAsset
            ? `${rasterAsset.width}px`
            : undefined,
          "--train-built-fixture-height": rasterAsset
            ? `${rasterAsset.height}px`
            : undefined,
          "--train-built-fixture-foundation-width": rasterAsset?.builtEnvironment
            ? `${rasterAsset.builtEnvironment.foundationWidthPx}px`
            : undefined,
        };
        return (
          <span
            className={[
              "train-built-environment-fixture",
              `train-built-environment-fixture--${fixture}`,
            ].join(" ")}
            data-built-fixture={fixture}
            data-built-fixture-owner={owner}
            data-built-fixture-surface="opaque"
            data-built-fixture-ground-height={groundHeight.toFixed(3)}
            data-built-fixture-contour-height={contourHeight.toFixed(3)}
            data-built-fixture-ground-inset={groundInset.toFixed(3)}
            data-built-fixture-foundation-error={(
              groundHeight +
              groundInset -
              contourHeight
            ).toFixed(3)}
            data-built-fixture-x-percent={xPercent}
            data-built-fixture-collision-width={collisionWidth.toFixed(3)}
            data-built-fixture-art={rasterAsset ? "raster" : "connector"}
            data-built-fixture-asset={rasterAsset?.id}
            data-built-fixture-pixel-density={
              rasterAsset?.builtEnvironment?.pixelDensity
            }
            data-built-fixture-perspective={
              rasterAsset?.builtEnvironment?.perspective
            }
            data-built-fixture-reference-module={
              rasterAsset?.builtEnvironment?.referenceModuleHeightPx
            }
            style={style}
            key={owner}
          >
            {rasterAsset ? (
              <>
                <span
                  className="train-built-environment-raster-foundation"
                  data-built-foundation-owner={owner}
                  data-built-foundation-contact="contour"
                />
                <img
                  className="train-built-environment-raster"
                  src={rasterAsset.src}
                  alt=""
                  aria-hidden="true"
                  draggable={false}
                  loading="lazy"
                  decoding="async"
                  width={rasterAsset.width}
                  height={rasterAsset.height}
                  data-built-raster-asset={rasterAsset.id}
                  data-built-raster-pixel-density={
                    rasterAsset.builtEnvironment?.pixelDensity
                  }
                />
                {rasterAsset.emissive ? (
                  <img
                    className={[
                      "train-emissive-overlay",
                      "train-built-environment-raster-emissive",
                    ].join(" ")}
                    src={rasterAsset.emissive.src}
                    alt=""
                    aria-hidden="true"
                    draggable={false}
                    loading="lazy"
                    decoding="async"
                    width={rasterAsset.emissive.width}
                    height={rasterAsset.emissive.height}
                    data-emissive="regional-fixture"
                    data-emissive-owner={owner}
                    data-emissive-region={beat.region}
                    data-emissive-schedule="sunset-night"
                  />
                ) : TRAIN_BUILT_ENVIRONMENT_EMISSIVE_FIXTURES.has(fixture) ? (
                  <span
                    className="train-emissive-overlay train-built-environment-fixture-emissive"
                    data-emissive="regional-fixture"
                    data-emissive-owner={owner}
                    data-emissive-region={beat.region}
                    data-emissive-schedule="sunset-night"
                  />
                ) : null}
                {fixture === "shop-awning" ? (
                  <span
                    className="train-built-environment-shop-awning"
                    data-built-fixture-detail="shop-awning"
                  />
                ) : null}
              </>
            ) : TRAIN_BUILT_ENVIRONMENT_EMISSIVE_FIXTURES.has(fixture) ? (
              <span
                className="train-emissive-overlay train-built-environment-fixture-emissive"
                data-emissive="regional-fixture"
                data-emissive-owner={owner}
                data-emissive-region={beat.region}
                data-emissive-schedule="sunset-night"
              />
            ) : null}
          </span>
        );
      })}
    </span>
  );
}

const TRAIN_COAST_FIXTURE_POSITIONS = [
  [18, 52, 79],
  [25, 68, 86],
  [14, 43, 74],
] as const;

const TRAIN_COAST_WATER_FIXTURES = new Set(["boat", "buoy"]);

function coastWaterOwner(chunk: RouteChunk, layer: TrainParallaxLayerName) {
  return (
    `coast:${chunk.seedVersion}:${chunk.routeSeed}:` +
    `${chunk.index}:${layer}:water`
  );
}

function TrainCoastComposition({
  beat,
  contour,
  chunk,
  layer,
}: {
  beat: TrainCoastSceneryBeat;
  contour: TrainTerrainContour;
  chunk: RouteChunk;
  layer: "midground";
}) {
  const positions =
    TRAIN_COAST_FIXTURE_POSITIONS[beat.templateVariant] ??
    TRAIN_COAST_FIXTURE_POSITIONS[0];
  const waterOwner = coastWaterOwner(chunk, layer);
  return (
    <span
      className={[
        "train-coast-composition",
        `train-coast-composition--${layer}`,
        `train-coast-composition--${beat.shoreFamily}`,
      ].join(" ")}
      data-coast-composition={beat.shoreFamily}
      data-coast-role={beat.role}
      data-coast-water-kind={beat.waterKind}
      data-coast-transition={beat.transition}
      data-coast-transition-neighbor={beat.transitionNeighbor ?? undefined}
      data-coast-transition-family={beat.transitionFamily}
      data-coast-rhythm-variant={beat.templateVariant}
      data-coast-layer-role="water-shore-fixtures"
      data-coast-single-owner="midground"
      aria-hidden="true"
    >
      <span
        className="train-coast-water-plane train-coast-water-plane--midground"
        data-water-plane="continuous"
        data-water-depth="midground"
        data-water-owner={waterOwner}
        data-water-surface="owned"
        data-coast-contact-medium="water"
        data-water-seam-left={contour.seamLeftHeightPx}
        data-water-seam-right={contour.seamRightHeightPx}
      >
        {[0, 1, 2].map((cue) => (
          <span
            className={`train-coast-water-cue train-coast-water-cue--${cue}`}
            data-water-movement-cue={cue}
            data-water-depth-owner={waterOwner}
            key={cue}
          />
        ))}
      </span>
      <span
        className="train-coast-shore-profile"
        data-shore-profile={beat.shoreFamily}
        data-shore-owner={`${chunk.index}:${layer}`}
        data-shore-continuity={`${contour.seamLeftHeightPx}:${contour.seamRightHeightPx}`}
        data-shore-surface="opaque"
        data-coast-contact-medium="dry-land"
        data-coast-dry-ground="shore-shelf"
      />
      {layer === "midground" ? (
        <span
          className="train-coast-fixtures"
          data-coast-fixture-count={beat.fixtures.length}
        >
          {beat.fixtures.map((fixture, fixtureIndex) => {
            const xPercent = positions[fixtureIndex] ?? 50;
            const groundHeight = trainTerrainHeightAtPercent(contour, xPercent);
            const waterFixture = TRAIN_COAST_WATER_FIXTURES.has(fixture);
            const waterlineHeight =
              47 + ((beat.templateVariant + fixtureIndex) % 3) * 3;
            const owner = `${chunk.index}:${fixtureIndex}:${fixture}`;
            const style: TrainCoastFixtureStyle = {
              left: `${xPercent}%`,
              "--train-coast-fixture-scale":
                0.88 + ((beat.templateVariant + fixtureIndex) % 3) * 0.08,
              "--train-coast-fixture-ground-height": `${groundHeight}px`,
              "--train-coast-waterline-height": `${waterlineHeight}px`,
            };
            return (
              <span
                className={[
                  "train-coast-fixture",
                  `train-coast-fixture--${fixture}`,
                ].join(" ")}
                data-coast-fixture={fixture}
                data-coast-fixture-owner={owner}
                data-coast-fixture-surface="opaque"
                data-coast-fixture-medium={
                  waterFixture ? "water" : "dry-land"
                }
                data-coast-fixture-ground-height={groundHeight.toFixed(3)}
                data-coast-fixture-waterline-height={
                  waterFixture ? waterlineHeight : undefined
                }
                data-water-owner={
                  waterFixture ? coastWaterOwner(chunk, "midground") : undefined
                }
                style={style}
                key={owner}
              >
                {fixture === "boat" ? (
                  <span
                    className="train-coast-boat-cabin"
                    data-coast-fixture-detail="boat-cabin"
                  />
                ) : null}
                {fixture === "pier" ? (
                  <span
                    className="train-coast-pier-piles"
                    data-coast-fixture-detail="pier-piles"
                  />
                ) : null}
              </span>
            );
          })}
        </span>
      ) : null}
    </span>
  );
}

const TRAIN_FOREST_GROUND_VEGETATION = new Map(
  TRAIN_SCENERY_VEGETATION.map((asset) => [asset.id, asset]),
);

function TrainForestGroundDetails({
  beat,
  chunk,
  contour,
}: {
  beat: TrainForestMountainSceneryBeat;
  chunk: RouteChunk;
  contour: TrainTerrainContour;
}) {
  const variant = (beat.templateVariant + chunk.variant) % 3;
  const positions =
    variant === 0 ? [14, 46, 81] : variant === 1 ? [24, 61, 88] : [9, 39, 72];
  const kinds =
    beat.role === "forest-clearing"
      ? (["tree-small", "meadow", "tree-small"] as const)
      : beat.role === "forest-stream"
        ? (["reeds", "tree-small", "reeds"] as const)
        : beat.role === "forest-undergrowth"
          ? (["tree-small", "shrub", "tree-small"] as const)
          : (["tree-tall", "shrub", "tree-small"] as const);
  const assetFor = (
    kind: (typeof kinds)[number],
    index: number,
  ): TrainSceneryAsset | null => {
    if (kind === "meadow") return null;
    const id =
      kind === "reeds"
        ? "vegetation-reeds"
        : kind === "shrub"
          ? "vegetation-hedgerow"
          : kind === "tree-tall"
            ? variant === 1
              ? "vegetation-deciduous"
              : "vegetation-conifer-tall"
            : (variant + index) % 2 === 0
              ? "vegetation-conifer-squat"
              : "vegetation-deciduous";
    return TRAIN_FOREST_GROUND_VEGETATION.get(id) ?? null;
  };

  return (
    <span
      className="train-forest-ground-details"
      data-forest-ground-role={beat.role}
      data-forest-ground-family={beat.silhouetteFamily}
      data-forest-ground-variant={variant}
      data-forest-ground-owner={`${chunk.index}:near`}
      aria-hidden="true"
    >
      {kinds.map((kind, index) => {
        const xPercent = positions[index]!;
        const groundHeight = trainTerrainHeightAtPercent(contour, xPercent);
        const asset = assetFor(kind, index);
        const scale =
          kind === "tree-tall"
            ? 0.66
            : kind === "tree-small"
              ? 0.58
              : kind === "shrub"
                ? 0.52
                : kind === "reeds"
                  ? 0.54
                  : 1;
        const style = {
          left: `${xPercent}%`,
          bottom: `${groundHeight}px`,
          "--train-forest-detail-scale":
            scale + ((variant + index) % 3) * 0.04,
        } as CSSProperties;
        if (asset) {
          return (
            <img
              className={[
                "train-forest-ground-detail",
                "train-forest-ground-detail--sprite",
                `train-forest-ground-detail--${kind}`,
                `train-forest-ground-detail--ordinal-${index}`,
              ].join(" ")}
              src={asset.src}
              alt=""
              aria-hidden="true"
              draggable={false}
              loading="lazy"
              decoding="async"
              width={asset.width}
              height={asset.height}
              data-forest-ground-detail={kind}
              data-forest-ground-asset={asset.id}
              data-forest-ground-anchor="contour"
              data-forest-ground-height={groundHeight.toFixed(3)}
              style={style}
              key={`${kind}-${index}`}
            />
          );
        }
        return (
          <span
            className={[
              "train-forest-ground-detail",
              `train-forest-ground-detail--${kind}`,
              `train-forest-ground-detail--ordinal-${index}`,
            ].join(" ")}
            data-forest-ground-detail={kind}
            data-forest-ground-anchor="contour"
            data-forest-ground-height={groundHeight.toFixed(3)}
            style={style}
            key={`${kind}-${index}`}
          />
        );
      })}
    </span>
  );
}

export const TrainRouteChunk = memo(function TrainRouteChunk({
  chunk: sourceChunk,
  layer,
  includeSetPieces = true,
  projection,
  suppressScenery = false,
}: {
  chunk: RouteChunk;
  layer: TrainParallaxLayer;
  includeSetPieces?: boolean;
  projection?: {
    coordinatePx: number;
    focus: TrainSetPieceFocus;
  };
  suppressScenery?: boolean;
}) {
  const chunk = includeSetPieces
    ? sourceChunk
    : { ...sourceChunk, setPiece: null };
  const diagnosticSetPiece = projection
    ? chunk.setPiece
    : sourceChunk.setPiece;
  const style: TrainRouteChunkStyle = {
    left: `${
      -(projection?.coordinatePx ?? chunk.index * TRAIN_ROUTE_CHUNK_WIDTH) -
      TRAIN_PARALLAX_SEAM_OVERLAP / 2
    }px`,
    width: `${TRAIN_ROUTE_CHUNK_WIDTH + TRAIN_PARALLAX_SEAM_OVERLAP}px`,
    "--train-chunk-terrain-height": `${chunk.terrainHeight}px`,
    "--train-chunk-ridge-height": `${chunk.ridgeHeight}px`,
    "--train-chunk-feature-offset": `${chunk.featureOffset}%`,
  };
  const sceneryPlacements = suppressScenery
    ? []
    : trainSceneryPlacementsForChunk(layer.name, chunk, {
        includeSetPieces,
      });
  const stationSegment =
    chunk.setPiece?.type === "station" ? chunk.setPiece : null;
  const traversalSegment =
    chunk.setPiece && TRAIN_TRAVERSAL_TYPES.has(chunk.setPiece.type)
      ? chunk.setPiece
      : null;
  const transitionSegment =
    chunk.setPiece && TRAIN_TRANSITION_TYPES.has(chunk.setPiece.type)
      ? chunk.setPiece
      : null;
  const rendersSetPiece =
    chunk.setPiece?.renderLayer === layer.name ||
    Boolean(
      (traversalSegment ?? transitionSegment)?.reservedLayers.includes(
        layer.name,
      ),
    );
  const stationComposition = stationSegment
    ? TRAIN_STATION_SEGMENT_COMPOSITIONS[stationSegment.segmentOffset]
    : null;
  const terrainContour =
    layer.name === "sky"
      ? null
      : trainTerrainContourForChunk(chunk, layer.name);
  const terrainStyle: TrainTerrainBaseStyle | undefined = terrainContour
    ? {
        "--train-terrain-point-0": `${terrainContour.points[0]!.heightPx}px`,
        "--train-terrain-point-1": `${terrainContour.points[1]!.heightPx}px`,
        "--train-terrain-point-2": `${terrainContour.points[2]!.heightPx}px`,
        "--train-terrain-point-3": `${terrainContour.points[3]!.heightPx}px`,
        "--train-terrain-point-4": `${terrainContour.points[4]!.heightPx}px`,
        "--train-terrain-point-5": `${terrainContour.points[5]!.heightPx}px`,
        "--train-terrain-point-6": `${terrainContour.points[6]!.heightPx}px`,
        "--train-terrain-point-7": `${terrainContour.points[7]!.heightPx}px`,
      }
    : undefined;
  const regionalSceneryBeat =
    trainForestMountainSceneryBeatForChunk(chunk);
  const builtEnvironmentBeat =
    trainTownIndustrialSceneryBeatForChunk(chunk);
  const coastBeat = trainCoastSceneryBeatForChunk(chunk);
  const regionalRole =
    regionalSceneryBeat?.role ?? builtEnvironmentBeat?.role ?? coastBeat?.role;
  const regionalFamily =
    regionalSceneryBeat?.silhouetteFamily ??
    builtEnvironmentBeat?.compositionFamily ??
    coastBeat?.shoreFamily;
  const regionalTemplateVariant =
    regionalSceneryBeat?.templateVariant ??
    builtEnvironmentBeat?.templateVariant ??
    coastBeat?.templateVariant;
  const regionalDensity =
    regionalSceneryBeat?.densityClass ??
    builtEnvironmentBeat?.densityClass ??
    coastBeat?.densityClass;
  const regionalTransition =
    regionalSceneryBeat?.transition ??
    builtEnvironmentBeat?.transition ??
    coastBeat?.transition;
  const regionalTransitionNeighbor =
    regionalSceneryBeat?.transitionNeighbor ??
    builtEnvironmentBeat?.transitionNeighbor ??
    coastBeat?.transitionNeighbor;

  return (
    <div
      className={[
        projection
          ? "train-set-piece-projection-segment"
          : "train-parallax-chunk",
        `train-parallax-chunk--${layer.name}`,
        `train-parallax-chunk--variant-${chunk.variant}`,
        layer.name === "near" && !projection ? "train-route-chunk" : "",
      ]
        .filter(Boolean)
        .join(" ")}
      data-route-chunk-index={chunk.index}
      data-route-chunk-variant={chunk.variant}
      data-route-region={chunk.region}
      data-route-region-index={chunk.regionIndex}
      data-route-region-offset={chunk.regionChunkOffset}
      data-regional-scenery-role={regionalRole}
      data-regional-silhouette={regionalFamily}
      data-regional-rhythm-variant={regionalTemplateVariant}
      data-regional-density={regionalDensity}
      data-regional-transition={regionalTransition}
      data-regional-transition-neighbor={
        regionalTransitionNeighbor ?? undefined
      }
      data-built-environment-ground={builtEnvironmentBeat?.groundKind}
      data-built-environment-family={builtEnvironmentBeat?.compositionFamily}
      data-built-environment-scale={builtEnvironmentBeat?.scaleFamily}
      data-coast-water-kind={coastBeat?.waterKind}
      data-coast-shore-family={coastBeat?.shoreFamily}
      data-regional-human-landmark={
        regionalSceneryBeat?.humanScaleLandmarkEligible
          ? "eligible"
          : regionalSceneryBeat
            ? "not-eligible"
            : undefined
      }
      data-route-set-piece={diagnosticSetPiece?.type ?? "none"}
      data-route-set-piece-role={diagnosticSetPiece?.role ?? "none"}
      data-route-set-piece-variant={
        diagnosticSetPiece?.visualVariant ?? "none"
      }
      data-route-set-piece-reserved-layers={
        diagnosticSetPiece?.reservedLayers.join(",") ?? ""
      }
      data-set-piece-projection={projection ? "journey-anchor" : undefined}
      data-set-piece-focus-id={projection?.focus.id}
      data-set-piece-focus-position={
        projection?.focus.journeyPosition.toFixed(3)
      }
      data-set-piece-segment-id={
        projection && chunk.setPiece
          ? `${chunk.setPiece.id}:${chunk.setPiece.segmentOffset}`
          : undefined
      }
      data-set-piece-reservation={
        projection && chunk.setPiece?.reservedLayers.includes(layer.name)
          ? chunk.setPiece.id
          : undefined
      }
      data-scenery-reserved={suppressScenery ? "projected-set-piece" : undefined}
      data-parallax-layer={layer.name}
      data-seam-overlap={TRAIN_PARALLAX_SEAM_OVERLAP}
      style={style}
    >
      {terrainContour && terrainStyle ? (
        <span
          className="train-terrain-base"
          data-terrain-owner="chunk-contour"
          data-terrain-region={terrainContour.region}
          data-terrain-variant={terrainContour.variant}
          data-terrain-layer={terrainContour.layer}
          data-terrain-material={terrainContour.material}
          data-terrain-transition-material={
            terrainContour.transitionMaterial ?? undefined
          }
          data-terrain-envelope={`${TRAIN_TERRAIN_LAYER_ENVELOPES[terrainContour.layer].minimumHeightPx}:${TRAIN_TERRAIN_LAYER_ENVELOPES[terrainContour.layer].maximumHeightPx}`}
          data-terrain-seam-left={terrainContour.seamLeftHeightPx}
          data-terrain-seam-right={terrainContour.seamRightHeightPx}
          data-terrain-contour={terrainContour.points
            .map((point) => `${point.xPercent}:${point.heightPx}`)
            .join(",")}
          data-coast-contact-medium={
            chunk.region === "coast" && layer.name === "midground"
              ? "dry-land"
              : undefined
          }
          data-coast-dry-ground={
            chunk.region === "coast" && layer.name === "midground"
              ? "terrain-contour"
              : undefined
          }
          aria-hidden="true"
          style={terrainStyle}
        >
          {builtEnvironmentBeat &&
          layer.name === "midground" &&
          !projection &&
          !suppressScenery ? (
            <span
              className={[
                "train-built-environment-ground",
                `train-built-environment-ground--${builtEnvironmentBeat.groundKind}`,
              ].join(" ")}
              data-built-ground={builtEnvironmentBeat.groundKind}
              data-built-ground-owner={`${chunk.index}:midground`}
              data-built-ground-surface="opaque"
            />
          ) : null}
        </span>
      ) : null}
      {regionalSceneryBeat?.region === "forest" &&
      terrainContour &&
      layer.name === "near" &&
      !projection &&
      !suppressScenery ? (
        <TrainForestGroundDetails
          beat={regionalSceneryBeat}
          chunk={chunk}
          contour={terrainContour}
        />
      ) : null}
      {coastBeat &&
      terrainContour &&
      layer.name === "midground" &&
      !projection &&
      !suppressScenery ? (
        <TrainCoastComposition
          beat={coastBeat}
          contour={terrainContour}
          chunk={chunk}
          layer={layer.name}
        />
      ) : null}
      {builtEnvironmentBeat &&
      terrainContour &&
      layer.name === "midground" &&
      !projection &&
      !suppressScenery ? (
        <TrainBuiltEnvironmentFixtures
          beat={builtEnvironmentBeat}
          contour={terrainContour}
          chunkIndex={chunk.index}
        />
      ) : null}
      {chunk.setPiece && rendersSetPiece ? (
        <>
          <span
            className={[
              "train-set-piece",
              `train-set-piece--${chunk.setPiece.type}`,
              `train-set-piece--${chunk.setPiece.role}`,
              `train-set-piece--variant-${chunk.setPiece.visualVariant}`,
              `train-set-piece--layer-${layer.name}`,
            ].join(" ")}
            data-set-piece-id={chunk.setPiece.id}
            data-set-piece-type={chunk.setPiece.type}
            data-set-piece-role={chunk.setPiece.role}
            data-set-piece-segment={chunk.setPiece.segmentOffset}
            data-set-piece-span={chunk.setPiece.span}
            data-set-piece-variant={chunk.setPiece.visualVariant}
            data-set-piece-start={chunk.setPiece.startIndex}
            data-set-piece-end={chunk.setPiece.endIndex}
            data-set-piece-occlusion="restrained"
            data-set-piece-layer={layer.name}
            data-set-piece-participation={
              layer.name === chunk.setPiece.renderLayer
                ? "primary"
                : "supporting"
            }
            data-town-edge-continuity={
              chunk.setPiece.type === "town-edge"
                ? `${chunk.setPiece.startIndex}:${chunk.setPiece.segmentOffset}`
                : undefined
            }
            style={
              {
                "--train-set-piece-phase": `${
                  -chunk.setPiece.segmentOffset * TRAIN_ROUTE_CHUNK_WIDTH
                }px`,
              } as TrainSetPieceStyle
            }
            data-station-assets={
              stationSegment
                ? [
                    "platform",
                    "campus",
                    stationComposition?.hasCanopy ? "canopy" : null,
                    "fixtures",
                    "service",
                  ]
                    .filter(Boolean)
                    .join(",")
                : undefined
            }
            data-station-vertical-zone={
              stationSegment ? "behind-train" : undefined
            }
          >
            {traversalSegment ? (
              <TrainTraversalComposition
                segment={traversalSegment}
                layer={layer.name}
              />
            ) : null}
            {chunk.setPiece.type === "town-edge" ? (
              <TrainTownEdgeComposition
                segment={chunk.setPiece}
                layer={layer.name}
              />
            ) : null}
            {chunk.setPiece.type === "coast-reveal" ? (
              <TrainCoastRevealComposition
                segment={chunk.setPiece}
                layer={layer.name}
              />
            ) : null}
            {stationSegment ? (
              <>
                <span
                  className={[
                    "train-station-transition",
                    `train-station-transition--${stationSegment.role}`,
                  ].join(" ")}
                  data-station-transition-geometry={stationSegment.role}
                  data-station-segment={stationSegment.segmentOffset}
                >
                  <span
                    className="train-station-platform"
                    data-station-asset="platform"
                    data-station-platform-contact="fixed-train-overlap"
                    data-station-solid-surface="opaque"
                  />
                </span>
                <span
                  className="train-station-architecture"
                  data-station-architecture="whole"
                  data-station-role={stationSegment.role}
                  data-station-segment={stationSegment.segmentOffset}
                  data-station-bay={stationComposition?.bay}
                  data-station-structure={stationComposition?.structure}
                  data-station-mass-role={stationComposition?.massRole}
                  data-station-opening={stationComposition?.opening ?? "none"}
                  data-station-negative-space={
                    stationComposition?.opening ?? "open-platform"
                  }
                  data-station-canopy-role={
                    stationComposition?.canopyRole ?? "none"
                  }
                >
                  {stationComposition?.hasBuilding ? (
                    <span
                      className={[
                        "train-station-building",
                        `train-station-building--${stationComposition.structure}`,
                      ].join(" ")}
                      data-station-asset="building"
                      data-station-building-kind={stationComposition.structure}
                      data-station-surface="opaque"
                      data-station-solid-surface="opaque"
                    >
                      {stationComposition.hasWindows ? (
                        <span
                          className="train-station-window-row"
                          data-station-asset="windows"
                          data-station-fixture="window-bank"
                        >
                          {stationComposition.windowLight ? (
                            <span
                              className={[
                                "train-emissive-overlay",
                                "train-station-emissive",
                                "train-station-window-emissive",
                              ].join(" ")}
                              data-emissive="station-windows"
                              data-emissive-owner={`station-segment-${stationSegment.segmentOffset}`}
                              data-station-light-schedule={
                                stationComposition.windowLight
                              }
                            />
                          ) : null}
                        </span>
                      ) : null}
                      {stationComposition.door ? (
                        <span
                          className={[
                            "train-station-door",
                            `train-station-door--${stationComposition.door}`,
                          ].join(" ")}
                          data-station-asset="door"
                          data-station-door={stationComposition.door}
                          data-station-entrance={stationComposition.entrance}
                        />
                      ) : null}
                      {stationSegment.segmentOffset === 2 ? (
                        <span
                          className="train-station-name-board"
                          data-station-asset="sign"
                        >
                          TMACT
                        </span>
                      ) : null}
                    </span>
                  ) : null}
                  {stationComposition?.opening ? (
                    <span
                      className={[
                        "train-station-open-air-bay",
                        `train-station-open-air-bay--${stationComposition.opening}`,
                      ].join(" ")}
                      data-station-framed-opening={stationComposition.opening}
                      data-station-opening-owner="world-parallax"
                    >
                      <span
                        className="train-station-open-air-view"
                        data-station-view-owner="world-parallax"
                      />
                    </span>
                  ) : null}
                  {stationComposition?.serviceElements.map((element, index) => (
                    <span
                      className={[
                        "train-station-service-element",
                        `train-station-service-element--${element}`,
                      ].join(" ")}
                      data-station-asset="service"
                      data-station-service-element={element}
                      data-station-service-slot={index}
                      key={`${element}-${index}`}
                    >
                      {element === "bench" ? (
                        <>
                          <span className="train-station-bench-seat" />
                          <span className="train-station-bench-leg" />
                        </>
                      ) : null}
                      {element === "baggage-cart" ? (
                        <>
                          <span className="train-station-cart-deck" />
                          <span className="train-station-cart-wheel train-station-cart-wheel--leading" />
                          <span className="train-station-cart-wheel train-station-cart-wheel--trailing" />
                        </>
                      ) : null}
                      {element === "parcel-stack" ? (
                        <>
                          <span className="train-station-parcel train-station-parcel--lower" />
                          <span className="train-station-parcel train-station-parcel--upper" />
                        </>
                      ) : null}
                    </span>
                  ))}
                  {stationComposition?.hasCanopy ? (
                    <span
                      className="train-station-canopy"
                      data-station-asset="canopy"
                      data-station-canopy-role={stationComposition.canopyRole}
                      data-station-surface="opaque"
                      data-station-solid-surface="opaque"
                    >
                      {stationComposition.supportSlots.map((slot) => (
                        <span
                          className={[
                            "train-station-canopy-support",
                            `train-station-canopy-support--${slot}`,
                          ].join(" ")}
                          data-station-asset="canopy-support"
                          data-station-fixture-slot={slot}
                          data-station-solid-surface="opaque"
                          key={slot}
                        />
                      ))}
                    </span>
                  ) : null}
                  {stationComposition?.lampSlots.map((slot) => (
                    <span
                      className={[
                        "train-station-lamp",
                        `train-station-lamp--${slot}`,
                      ].join(" ")}
                      data-station-asset="lamp"
                      data-station-fixture-slot={slot}
                      data-station-light-schedule={
                        stationSegment.segmentOffset === 2
                          ? "sunset-night"
                          : "night"
                      }
                      key={slot}
                    >
                      <span
                        className="train-station-lamp-fixture"
                        data-station-fixture="lamp-head"
                        data-station-solid-surface="opaque"
                      />
                      <span
                        className={[
                          "train-emissive-overlay",
                          "train-station-emissive",
                          "train-station-lamp-emissive",
                        ].join(" ")}
                        data-emissive="station-lamp"
                        data-emissive-owner={`station-segment-${stationSegment.segmentOffset}-${slot}`}
                        data-station-light-schedule={
                          stationSegment.segmentOffset === 2
                            ? "sunset-night"
                            : "night"
                        }
                      />
                    </span>
                  ))}
                </span>
              </>
            ) : null}
          </span>
          {stationSegment && stationComposition?.signalAspect ? (
            <span
              className="train-station-signal"
              data-station-asset="signal"
              data-station-signal-aspect={stationComposition.signalAspect}
              data-station-owner-segment={stationSegment.segmentOffset}
            >
              <span
                className="train-station-signal-head"
                data-station-fixture="signal-head"
                data-station-solid-surface="opaque"
              />
              <span
                className={[
                  "train-emissive-overlay",
                  "train-station-emissive",
                  "train-station-signal-emissive",
                ].join(" ")}
                data-emissive="station-signal"
                data-emissive-owner={`station-segment-${stationSegment.segmentOffset}`}
                data-station-light-schedule="sunset-night"
              />
            </span>
          ) : null}
          {stationSegment?.segmentOffset === 2 ? (
            <span
              className="train-station-ambient-steam"
              data-station-ambient-detail="steam"
            />
          ) : null}
        </>
      ) : null}
      {sceneryPlacements.map((placement, ordinal) => {
        const { asset } = placement;
        const coastLandOwned =
          chunk.region === "coast" &&
          layer.name !== "sky" &&
          asset.category !== "cloud" &&
          asset.category !== "coast";
        const needsFoundation =
          Boolean(asset.builtEnvironment) ||
          (coastLandOwned && asset.category === "building");
        const sceneryContourHeight = terrainContour
          ? trainTerrainHeightAtPercent(
              terrainContour,
              placement.offsetPercent,
            )
          : undefined;
        const sceneryGroundHeight =
          sceneryContourHeight === undefined
            ? undefined
            : sceneryContourHeight - placement.groundInsetPx;
        const anchorError =
          sceneryContourHeight === undefined || sceneryGroundHeight === undefined
            ? undefined
            : sceneryGroundHeight +
              placement.groundInsetPx -
              sceneryContourHeight;
        const sceneryInstanceId =
          `${chunk.seedVersion}:${chunk.routeSeed}:${layer.name}:` +
          `${chunk.index}:${ordinal}:${asset.id}`;
        const nightLife = trainNightLifeForPlacement(
          chunk,
          placement,
          ordinal,
        );
        const sceneryStyle: TrainSceneryAssetStyle = {
          left: `${placement.offsetPercent}%`,
          top:
            placement.altitudePercent === undefined
              ? undefined
              : `${placement.altitudePercent}%`,
          "--train-scenery-scale": placement.scale,
          "--train-scenery-ground-height":
            sceneryGroundHeight === undefined
              ? undefined
              : `${sceneryGroundHeight}px`,
          "--train-scenery-foundation-width": asset.builtEnvironment
            ? `${asset.builtEnvironment.foundationWidthPx}px`
            : coastLandOwned && asset.category === "building"
              ? `${asset.collisionWidth}px`
            : undefined,
          "--train-cloud-drift-start":
            placement.cloudDriftDistancePx === undefined
              ? undefined
              : `${(
                  placement.cloudDriftDistancePx *
                  (placement.cloudDriftDirection ?? 1) *
                  -1
                ).toFixed(3)}px`,
          "--train-cloud-drift-end":
            placement.cloudDriftDistancePx === undefined
              ? undefined
              : `${(
                  placement.cloudDriftDistancePx *
                  (placement.cloudDriftDirection ?? 1)
                ).toFixed(3)}px`,
          "--train-cloud-drift-duration":
            placement.cloudDriftDurationMs === undefined
              ? undefined
              : `${placement.cloudDriftDurationMs}ms`,
          "--train-cloud-drift-delay":
            placement.cloudDriftPhaseMs === undefined
              ? undefined
              : `${-placement.cloudDriftPhaseMs}ms`,
        };
        const sprites = [
          ...(needsFoundation
            ? [
                <span
                  className="train-scenery-foundation"
                  aria-hidden="true"
                  data-built-foundation-owner={sceneryInstanceId}
                  data-built-foundation-contact="contour"
                  data-built-foundation-ground-height={
                    sceneryGroundHeight?.toFixed(3)
                  }
                  data-built-foundation-contour-height={
                    sceneryContourHeight?.toFixed(3)
                  }
                  data-built-foundation-inset={placement.groundInsetPx.toFixed(
                    3,
                  )}
                  data-built-foundation-error={anchorError?.toFixed(3)}
                  data-coast-contact-medium={
                    coastLandOwned ? "dry-land" : undefined
                  }
                  data-coast-foundation={
                    coastLandOwned ? "opaque-shelf" : undefined
                  }
                  style={sceneryStyle}
                  key={`foundation-${asset.id}-${ordinal}`}
                />,
              ]
            : []),
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
            data-scenery-instance-id={sceneryInstanceId}
            data-scenery-category={asset.category}
            data-scenery-manifest-layer={asset.layer}
            data-scenery-anchor={asset.anchor}
            data-scenery-safe-scale={asset.safeScale.join("-")}
            data-scenery-asset-scale={placement.assetScale.toFixed(3)}
            data-scenery-depth-scale={placement.depthScaleMultiplier.toFixed(3)}
            data-scenery-detail-budget={placement.detailBudget}
            data-scenery-day-night={asset.dayNightTreatment}
            data-scenery-pixel-density={
              asset.builtEnvironment?.pixelDensity
            }
            data-scenery-perspective={asset.builtEnvironment?.perspective}
            data-scenery-reference-module={
              asset.builtEnvironment?.referenceModuleHeightPx
            }
            data-scenery-foundation-width={
              asset.builtEnvironment?.foundationWidthPx
            }
            data-scenery-landmark={placement.landmark ? "true" : "false"}
            data-scenery-regional-role={placement.regionalRole}
            data-scenery-silhouette={placement.silhouetteFamily}
            data-scenery-rhythm-variant={placement.regionalTemplateVariant}
            data-scenery-transition={placement.regionalTransition}
            data-scenery-scale-family={placement.regionalScaleFamily}
            data-scenery-water-kind={placement.regionalWaterKind}
            data-coast-contact-medium={
              coastLandOwned ? "dry-land" : undefined
            }
            data-scenery-set-piece={placement.setPiece?.type ?? "none"}
            data-scenery-set-piece-role={placement.setPiece?.role ?? "none"}
            data-scenery-set-piece-variant={
              placement.setPiece?.visualVariant ?? "none"
            }
            data-scenery-collision-width={placement.collisionWidth.toFixed(3)}
            data-scenery-minimum-spacing={placement.minimumSpacingPx}
            data-scenery-ground-height={
              sceneryGroundHeight?.toFixed(3) ?? undefined
            }
            data-scenery-contour-height={
              sceneryContourHeight?.toFixed(3) ?? undefined
            }
            data-scenery-ground-inset={placement.groundInsetPx.toFixed(3)}
            data-scenery-anchor-error={anchorError?.toFixed(3)}
            data-scenery-overlap-limit={
              placement.maximumCollisionOverlapRatio.toFixed(3)
            }
            data-scenery-terrain-owner={
              terrainContour ? `${chunk.index}:${terrainContour.layer}` : undefined
            }
            data-cloud-altitude={
              placement.altitudePercent?.toFixed(3) ?? undefined
            }
            data-cloud-pattern={placement.cloudPattern}
            data-cloud-group={placement.cloudGroup || undefined}
            data-cloud-rendering={
              asset.category === "cloud" ? "palette-specific" : undefined
            }
            data-cloud-route-position={
              placement.routePositionPx?.toFixed(3) ?? undefined
            }
            data-cloud-drift-distance={
              placement.cloudDriftDistancePx?.toFixed(3) ?? undefined
            }
            data-cloud-drift-direction={placement.cloudDriftDirection}
            data-cloud-drift-duration={placement.cloudDriftDurationMs}
            data-cloud-drift-phase={placement.cloudDriftPhaseMs}
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
              data-emissive-owner-instance={sceneryInstanceId}
              data-emissive-region={chunk.region}
              data-emissive-plane="owner-attached"
              data-emissive-enabled={nightLife ? "true" : "false"}
              data-emissive-occupancy={nightLife?.occupancy ?? "none"}
              data-emissive-load="pending"
              data-scenery-anchor={asset.anchor}
              data-scenery-manifest-layer={asset.layer}
              data-emissive-ground-height={
                sceneryGroundHeight?.toFixed(3) ?? undefined
              }
              data-emissive-contour-height={
                sceneryContourHeight?.toFixed(3) ?? undefined
              }
              data-emissive-ground-inset={placement.groundInsetPx.toFixed(3)}
              data-emissive-scale={placement.scale.toFixed(3)}
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
        if (nightLife && !asset.emissive) {
          sprites.push(
            <TrainNightLife
              plan={nightLife}
              placement={placement}
              groundHeight={sceneryGroundHeight}
              contourHeight={sceneryContourHeight}
              instanceId={sceneryInstanceId}
              waterOwner={
                chunk.region === "coast"
                  ? coastWaterOwner(chunk, "midground")
                  : undefined
              }
              key={`night-life-${asset.id}-${ordinal}`}
            />,
          );
        }
        return sprites;
      })}
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

interface TrainProjectedSetPieceSegment {
  chunk: RouteChunk;
  coordinatePx: number;
  focus: TrainSetPieceFocus;
}

type TrainProjectedSetPieceLayers = Record<
  TrainParallaxLayerName,
  readonly TrainProjectedSetPieceSegment[]
>;

function projectedSetPieceSegmentsForLayer(
  seed: string,
  layer: TrainParallaxLayer,
  routePosition: number,
  viewportWidth: number,
): readonly TrainProjectedSetPieceSegment[] {
  if (layer.name === "sky" || layer.name === "ultra-far") return [];
  const maximumCompositionWidth =
    TRAIN_ROUTE_CHUNK_WIDTH *
    Math.max(
      ...Object.values(TRAIN_SET_PIECE_DEFINITIONS).map(
        (definition) => definition.span,
      ),
    );
  const journeyMargin =
    (viewportWidth / 2 + maximumCompositionWidth) / layer.speedRatio;
  const firstChunk =
    Math.floor(
      (routePosition - journeyMargin - viewportWidth / 2) /
        TRAIN_ROUTE_CHUNK_WIDTH,
    ) - 1;
  const lastChunk =
    Math.ceil(
      (routePosition + journeyMargin - viewportWidth / 2) /
        TRAIN_ROUTE_CHUNK_WIDTH,
    ) + 1;
  const seen = new Set<string>();
  const projected: TrainProjectedSetPieceSegment[] = [];

  for (let chunkIndex = firstChunk; chunkIndex <= lastChunk; chunkIndex++) {
    const entry = generateRouteChunk(seed, chunkIndex).setPiece;
    if (
      !entry ||
      entry.role !== "entry" ||
      seen.has(entry.id) ||
      !entry.reservedLayers.includes(layer.name)
    ) {
      continue;
    }
    seen.add(entry.id);
    const focus = trainSetPieceFocusFromSegment(entry, viewportWidth);
    const geometry = trainSetPieceScreenGeometry(
      focus,
      layer.speedRatio,
      routePosition,
    );
    if (
      geometry.screenRightPx < -TRAIN_ROUTE_CHUNK_WIDTH ||
      geometry.screenLeftPx > viewportWidth + TRAIN_ROUTE_CHUNK_WIDTH
    ) {
      continue;
    }
    for (let offset = 0; offset < entry.span; offset++) {
      const segmentIndex = entry.startIndex + offset;
      projected.push({
        chunk: generateRouteChunk(seed, segmentIndex),
        coordinatePx: trainSetPieceProjectedCoordinate(
          focus,
          segmentIndex,
          layer.speedRatio,
        ),
        focus,
      });
    }
  }
  return projected;
}

function projectedSetPieceGeometry(
  focus: TrainSetPieceFocus,
  routePosition: number,
) {
  const layer = TRAIN_PARALLAX_LAYERS.find(
    (candidate) => candidate.name === focus.renderLayer,
  )!;
  return trainSetPieceScreenGeometry(
    focus,
    layer.speedRatio,
    routePosition,
  );
}

export function resolveTrainProjectedSetPieceCollisions(
  projected: TrainProjectedSetPieceLayers,
  routePosition: number,
  viewportWidth: number,
  stationState: string,
  prioritizedFocusID: string | null = null,
): {
  layers: TrainProjectedSetPieceLayers;
  excludedIDs: readonly string[];
} {
  const focuses = new Map<string, TrainSetPieceFocus>();
  for (const layer of TRAIN_PARALLAX_LAYERS) {
    for (const segment of projected[layer.name]) {
      focuses.set(segment.focus.id, segment.focus);
    }
  }
  const geometry = new Map(
    [...focuses.values()].map((focus) => [
      focus.id,
      projectedSetPieceGeometry(focus, routePosition),
    ]),
  );
  const transitionThreshold = Math.min(320, viewportWidth * 0.5);
  const transitions = [...focuses.values()]
    .filter(
      (focus) =>
        TRAIN_TRANSITION_TYPES.has(focus.type) &&
        geometry.get(focus.id)!.visibleWidthPx >= transitionThreshold,
    )
    .sort((left, right) => {
      const widthDifference =
        geometry.get(right.id)!.visibleWidthPx -
        geometry.get(left.id)!.visibleWidthPx;
      if (widthDifference !== 0) return widthDifference;
      if (left.type === right.type) return left.startIndex - right.startIndex;
      return left.type === "coast-reveal" ? -1 : 1;
    });
  const excludedIDs = new Set<string>();

  for (const transition of transitions) {
    if (excludedIDs.has(transition.id)) continue;
    const transitionGeometry = geometry.get(transition.id)!;
    for (const candidate of focuses.values()) {
      if (
        candidate.id === transition.id ||
        excludedIDs.has(candidate.id) ||
        !trainSetPiecesAreIncompatible(transition.type, candidate.type)
      ) {
        continue;
      }
      const candidateGeometry = geometry.get(candidate.id)!;
      const overlaps =
        transitionGeometry.visibleLeftPx < candidateGeometry.visibleRightPx &&
        transitionGeometry.visibleRightPx > candidateGeometry.visibleLeftPx;
      if (!overlaps) continue;
      if (candidate.id === prioritizedFocusID) {
        excludedIDs.add(transition.id);
        break;
      }
      if (transition.id === prioritizedFocusID) {
        excludedIDs.add(candidate.id);
        continue;
      }
      if (candidate.type === "station" && stationState !== "cruise") {
        excludedIDs.add(transition.id);
        break;
      }
      excludedIDs.add(candidate.id);
    }
  }

  const layers = { ...projected };
  for (const layer of TRAIN_PARALLAX_LAYERS) {
    layers[layer.name] = projected[layer.name].filter(
      (segment) => !excludedIDs.has(segment.focus.id),
    );
  }
  return { layers, excludedIDs: [...excludedIDs].sort() };
}

function usePrefersReducedTrainMotion(): boolean {
  const forced = trainWorldReducedMotionForced(window.location.search);
  const [reducedMotion, setReducedMotion] = useState(
    () =>
      forced ||
      (typeof window.matchMedia === "function" &&
        window.matchMedia("(prefers-reduced-motion: reduce)").matches),
  );

  useEffect(() => {
    if (forced) return;
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(query.matches);
    query.addEventListener?.("change", update);
    return () => query.removeEventListener?.("change", update);
  }, [forced]);

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
  const [journeyStorage] = useState(trainJourneyStorage);
  const [initialJourney] = useState(() => {
    const search = window.location.search;
    const parameters = new URLSearchParams(search);
    const stored = loadTrainJourneySnapshot(
      journeyStorage,
      TRAIN_ROUTE_SEED_VERSION,
    );
    const requestedSeed = parameters.get(TRAIN_WORLD_SEED_PARAM)?.trim();
    const hasSeedOverride = import.meta.env.DEV && Boolean(requestedSeed);
    const hasPositionOverride =
      import.meta.env.DEV && parameters.has(TRAIN_WORLD_POSITION_PARAM);
    const trigger = trainStationDevelopmentTrigger(search);
    const seed = hasSeedOverride
      ? trainWorldRouteSeed(search)
      : stored?.routeSeed ?? trainWorldRouteSeed(search);
    const focusOverride = trainWorldSetPieceFocus(
      search,
      seed,
      initialWorldWidth(),
    );
    const restoreCandidate =
      !hasPositionOverride &&
      focusOverride === null &&
      trigger === null &&
      stored?.routeSeed === seed
        ? createTrainStationJourney(
            seed,
            stored.routePosition,
            { cruiseSpeed },
            null,
          )
        : null;
    const canRestore = restoreCandidate?.state === "cruise";
    const routePosition = hasPositionOverride
      ? trainWorldRoutePosition(search)
      : focusOverride
        ? focusOverride.journeyPosition
      : canRestore && restoreCandidate
        ? restoreCandidate.routePosition
        : 0;
    return {
      restored: canRestore,
      restoredSnapshot: canRestore ? stored : null,
      focusOverride,
      stationJourney:
        canRestore && restoreCandidate
          ? restoreCandidate
          : createTrainStationJourney(
              seed,
              routePosition,
              { cruiseSpeed },
              trigger,
            ),
    };
  });
  const initialStationJourney = initialJourney.stationJourney;
  const seed = initialStationJourney.seed;
  const [routeEngines] = useState(() => createRouteEngines(seed));
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
  const [nightSkyCatalogue, setNightSkyCatalogue] = useState(() =>
    generateTrainNightSkyCatalogue(seed, initialWorldWidth()),
  );
  const [daySkyCatalogue, setDaySkyCatalogue] = useState(() =>
    generateTrainDaySkyCatalogue(seed, initialWorldWidth()),
  );
  const nightSkyViewportWidthRef = useRef(nightSkyCatalogue.viewportWidth);
  const routeWindowsRef = useRef(routeWindows);
  const stationJourneyRef = useRef(initialStationJourney);
  const routePositionRef = useRef(initialStationJourney.routePosition);
  const safeCheckpointRef = useRef<TrainJourneySnapshot | null>(
    trainJourneyCheckpoint(initialStationJourney, TRAIN_ROUTE_SEED_VERSION),
  );
  const persistedCheckpointRef = useRef<TrainJourneySnapshot | null>(
    initialJourney.restoredSnapshot,
  );
  const lastPersistenceAttemptRef = useRef(Date.now());
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
    const persistJourney = (force = false) => {
      const checkpoint = safeCheckpointRef.current;
      if (!checkpoint) return;
      const previous = persistedCheckpointRef.current;
      if (
        previous?.seedVersion === checkpoint.seedVersion &&
        previous.routeSeed === checkpoint.routeSeed &&
        previous.routePosition === checkpoint.routePosition
      ) {
        return;
      }
      const now = Date.now();
      if (
        !force &&
        !trainJourneyPersistenceDue(lastPersistenceAttemptRef.current, now)
      ) {
        return;
      }
      lastPersistenceAttemptRef.current = now;
      const saved = writeTrainJourneySnapshot(journeyStorage, checkpoint);
      world.dataset.journeyPersistence = saved ? "saved" : "unavailable";
      if (saved) persistedCheckpointRef.current = checkpoint;
    };
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
      const safeCheckpoint = trainJourneyCheckpoint(
        stationJourney,
        TRAIN_ROUTE_SEED_VERSION,
      );
      if (safeCheckpoint) safeCheckpointRef.current = safeCheckpoint;
      const checkpointPosition = safeCheckpointRef.current?.routePosition;
      world.dataset.journeyCheckpointPosition =
        checkpointPosition === undefined
          ? "none"
          : `${checkpointPosition.toFixed(3)}px`;
      persistJourney();
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
      const nightSkyViewportWidth = Math.max(1, Math.round(width));
      if (nightSkyViewportWidthRef.current !== nightSkyViewportWidth) {
        nightSkyViewportWidthRef.current = nightSkyViewportWidth;
        setNightSkyCatalogue(
          generateTrainNightSkyCatalogue(seed, nightSkyViewportWidth),
        );
        setDaySkyCatalogue(
          generateTrainDaySkyCatalogue(seed, nightSkyViewportWidth),
        );
      }
      for (const anchor of world.querySelectorAll<HTMLElement>(
        "[data-day-sky-anchor]",
      )) {
        const initialXPercent = Number.parseFloat(
          anchor.dataset.skySeedX ?? "",
        );
        const speedRatio = Number.parseFloat(
          anchor.dataset.skySpeedRatio ?? "",
        );
        const position = trainSkyAnchorPositionPx(
          routePosition,
          speedRatio,
          width,
          initialXPercent,
          reducedMotion,
        );
        const motionDistance = reducedMotion ? 0 : routePosition * speedRatio;
        anchor.style.setProperty(
          "--train-sky-anchor-x",
          `${position.toFixed(3)}px`,
        );
        anchor.dataset.skyPosition = `${position.toFixed(3)}px`;
        anchor.dataset.skyMotionDistance =
          `${motionDistance.toFixed(3)}px`;
      }
      let windowsChanged = false;
      const nextWindows = { ...routeWindowsRef.current };

      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const layerPosition = trainParallaxLayerPosition(
          routePosition,
          layer.speedRatio,
          reducedMotion && layer.name === "sky",
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
        persistJourney(true);
        setMotionState("suspended");
        world.dataset.stationAmbient = "suspended";
        return;
      }
      scheduleMotion();
    };
    const handlePageHide = () => persistJourney(true);

    applyRoutePosition();
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(applyRoutePosition);
    observer?.observe(world);
    const handleResize = applyRoutePosition;
    window.addEventListener("resize", handleResize);
    window.addEventListener("pagehide", handlePageHide);
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
      window.removeEventListener("pagehide", handlePageHide);
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [cruiseSpeed, journeyStorage, reducedMotion, routeEngines, seed]);

  const nearWindow = routeWindows.near;
  const projectionViewportWidth = nearWindow.viewportWidth;
  const rawProjectedSetPieces = Object.fromEntries(
    TRAIN_PARALLAX_LAYERS.map((layer) => [
      layer.name,
      projectedSetPieceSegmentsForLayer(
        seed,
        layer,
        routePositionRef.current,
        projectionViewportWidth,
      ),
    ]),
  ) as TrainProjectedSetPieceLayers;
  const projectedCollisionResolution =
    resolveTrainProjectedSetPieceCollisions(
      rawProjectedSetPieces,
      routePositionRef.current,
      projectionViewportWidth,
      stationJourneyRef.current.state,
      initialJourney.focusOverride?.id ?? null,
    );
  const projectedSetPieces = projectedCollisionResolution.layers;
  const projectedSetPieceCount = TRAIN_PARALLAX_LAYERS.reduce(
    (total, layer) => total + projectedSetPieces[layer.name].length,
    0,
  );

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
      data-journey-snapshot-version={TRAIN_JOURNEY_SNAPSHOT_VERSION}
      data-journey-restored={initialJourney.restored ? "true" : "false"}
      data-journey-persistence={
        journeyStorage ? (initialJourney.restored ? "restored" : "ready") : "unavailable"
      }
      data-journey-checkpoint-position={
        safeCheckpointRef.current
          ? `${safeCheckpointRef.current.routePosition.toFixed(3)}px`
          : "none"
      }
      data-route-chunk-indices={nearWindow.chunks
        .map((chunk) => chunk.index)
        .join(",")}
      data-route-mounted-chunks={nearWindow.chunks.length}
      data-route-total-mounted-chunks={totalMountedRouteChunks(routeWindows)}
      data-projected-set-piece-segments={projectedSetPieceCount}
      data-set-piece-collision-exclusions={
        projectedCollisionResolution.excludedIDs.length
      }
      data-set-piece-collision-excluded-ids={
        projectedCollisionResolution.excludedIDs.join(",")
      }
      data-set-piece-focus-type={initialJourney.focusOverride?.type}
      data-set-piece-focus-id={initialJourney.focusOverride?.id}
      data-set-piece-focus-position={
        initialJourney.focusOverride?.journeyPosition.toFixed(3)
      }
      data-set-piece-focus-segments={
        initialJourney.focusOverride?.expectedVisibleSegmentIDs.join(",")
      }
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
      data-star-count={nightSkyCatalogue.stars.length}
      data-star-viewport-width={nightSkyCatalogue.viewportWidth}
      data-night-sky-count={nightSkyCatalogue.elementCount}
      data-night-sky-version={nightSkyCatalogue.version}
      data-day-sky-count={daySkyCatalogue.elementCount}
      data-day-sky-weather={daySkyCatalogue.weather}
      data-day-sky-viewport-width={daySkyCatalogue.viewportWidth}
      data-sky-motion="route-derived"
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
        data-star-count={nightSkyCatalogue.stars.length}
        data-star-viewport-width={nightSkyCatalogue.viewportWidth}
        data-night-sky-count={nightSkyCatalogue.elementCount}
        aria-hidden="true"
      >
        <span
          className={[
            "train-day-sky",
            `train-day-sky--${daySkyCatalogue.weather}`,
          ].join(" ")}
          data-day-sky-catalogue={daySkyCatalogue.seed}
          data-day-sky-count={daySkyCatalogue.elementCount}
          data-day-sky-weather={daySkyCatalogue.weather}
          data-day-sky-negative-space={`${daySkyCatalogue.negativeSpaceStartPercent.toFixed(
            3,
          )}-${daySkyCatalogue.negativeSpaceEndPercent.toFixed(3)}`}
          data-sky-plane="behind-terrain"
          data-control-contrast="preserved"
          data-motion={reducedMotion ? "reduced" : "full"}
        >
          {[daySkyCatalogue.sun, ...daySkyCatalogue.wisps].map(
            (anchor: TrainDaySkyAnchor) => {
              const speedRatio =
                anchor.kind === "sun"
                  ? TRAIN_SKY_SUN_SPEED_RATIO
                  : TRAIN_SKY_WISP_SPEED_RATIO;
              const position = trainSkyAnchorPositionPx(
                routePositionRef.current,
                speedRatio,
                daySkyCatalogue.viewportWidth,
                anchor.xPercent,
                reducedMotion,
              );
              const style: TrainDaySkyAnchorStyle = {
                left: "0px",
                "--train-sky-anchor-opacity": anchor.opacity,
                "--train-sky-anchor-width": `${anchor.widthPx.toFixed(3)}px`,
                "--train-sky-anchor-height": `${anchor.heightPx.toFixed(3)}px`,
                "--train-sky-anchor-x": `${position.toFixed(3)}px`,
                "--train-sky-anchor-day-y": `${anchor.yPercent.toFixed(3)}%`,
                "--train-sky-anchor-sunset-y":
                  `${anchor.sunsetYPercent.toFixed(3)}%`,
              };
              return (
                <i
                  className={`train-day-sky-anchor train-day-sky-anchor--${anchor.kind}`}
                  data-day-sky-anchor={anchor.kind}
                  data-day-sky-anchor-id={anchor.id}
                  data-sky-seed-x={anchor.xPercent.toFixed(3)}
                  data-sky-day-y={anchor.yPercent.toFixed(3)}
                  data-sky-sunset-y={anchor.sunsetYPercent.toFixed(3)}
                  data-sky-speed-ratio={speedRatio}
                  data-sky-position={`${position.toFixed(3)}px`}
                  data-sky-motion-distance={`${(
                    reducedMotion
                      ? 0
                      : routePositionRef.current * speedRatio
                  ).toFixed(3)}px`}
                  style={style}
                  key={anchor.id}
                />
              );
            },
          )}
        </span>
        <span
          className="train-night-sky"
          data-night-sky-catalogue={nightSkyCatalogue.seed}
          data-night-sky-version={nightSkyCatalogue.version}
          data-night-sky-count={nightSkyCatalogue.elementCount}
          data-sky-plane="behind-terrain"
          data-control-contrast="preserved"
          data-motion={reducedMotion ? "reduced" : "full"}
        >
          {nightSkyCatalogue.band ? (
            <i
              className="train-emissive-overlay train-celestial-band"
              data-celestial-band={nightSkyCatalogue.band.id}
              data-emissive="airglow"
              style={
                {
                  top: `${nightSkyCatalogue.band.yPercent}%`,
                  "--train-celestial-band-height":
                    `${nightSkyCatalogue.band.heightPx.toFixed(3)}px`,
                  "--train-celestial-band-opacity":
                    nightSkyCatalogue.band.opacity,
                  "--train-celestial-band-rotation":
                    `${nightSkyCatalogue.band.rotationDeg.toFixed(3)}deg`,
                } as TrainCelestialBandStyle
              }
            />
          ) : null}
          <span
            className="train-emissive-overlay train-emissive-overlay--stars"
            data-emissive="stars"
            data-star-catalogue={nightSkyCatalogue.seed}
            data-star-count={nightSkyCatalogue.stars.length}
            data-star-target-count={nightSkyCatalogue.targetCount}
            data-star-negative-space={`${nightSkyCatalogue.negativeSpaceStartPercent.toFixed(
              3,
            )}-${nightSkyCatalogue.negativeSpaceEndPercent.toFixed(3)}`}
            data-motion={reducedMotion ? "reduced" : "full"}
          >
            {nightSkyCatalogue.stars.map((star: TrainStar) => {
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
            className={[
              "train-emissive-overlay",
              "train-emissive-overlay--moon",
              `train-emissive-overlay--moon-${nightSkyCatalogue.moon.phase}`,
              `train-emissive-overlay--moon-${nightSkyCatalogue.moon.direction}`,
            ].join(" ")}
            data-emissive="moon"
            data-moon-id={nightSkyCatalogue.moon.id}
            data-moon-phase={nightSkyCatalogue.moon.phase}
            data-moon-direction={nightSkyCatalogue.moon.direction}
            data-moon-exclusion={`${nightSkyCatalogue.moon.exclusionRadiusXPercent.toFixed(
              3,
            )}x${nightSkyCatalogue.moon.exclusionRadiusYPercent.toFixed(3)}`}
            style={
              {
                left: `${nightSkyCatalogue.moon.xPercent}%`,
                top: `${nightSkyCatalogue.moon.yPercent}%`,
                "--train-moon-size":
                  `${nightSkyCatalogue.moon.diameterPx.toFixed(3)}px`,
                "--train-moon-shadow-offset":
                  nightSkyCatalogue.moon.direction === "waxing"
                    ? "-18%"
                    : "18%",
                "--train-moon-shadow-opacity":
                  nightSkyCatalogue.moon.phase === "full" ? 0 : 1,
                "--train-moon-shadow-scale":
                  nightSkyCatalogue.moon.phase === "crescent"
                    ? 0.94
                    : nightSkyCatalogue.moon.phase === "quarter"
                      ? 1.45
                      : 0.54,
              } as TrainMoonStyle
            }
          />
          {nightSkyCatalogue.accent ? (
            <i
              className={[
                "train-emissive-overlay",
                "train-celestial-accent",
                `train-celestial-accent--${nightSkyCatalogue.accent.kind}`,
              ].join(" ")}
              data-celestial-accent={nightSkyCatalogue.accent.kind}
              data-emissive="celestial-accent"
              style={
                {
                  left: `${nightSkyCatalogue.accent.xPercent}%`,
                  top: `${nightSkyCatalogue.accent.yPercent}%`,
                  width: `${nightSkyCatalogue.accent.widthPx.toFixed(3)}px`,
                  height: `${nightSkyCatalogue.accent.heightPx.toFixed(3)}px`,
                  transform:
                    `translate(-50%, -50%) rotate(${nightSkyCatalogue.accent.rotationDeg.toFixed(3)}deg)`,
                  "--train-celestial-accent-opacity":
                    nightSkyCatalogue.accent.opacity,
                } as TrainCelestialAccentStyle
              }
            />
          ) : null}
        </span>
      </div>
      {TRAIN_PARALLAX_LAYERS.map((layer, layerIndex) => {
        const layerWindow = routeWindows[layer.name];
        const depth = TRAIN_SCENERY_DEPTH_PROFILES[layer.name];
        const initialLayerPosition = trainParallaxLayerPosition(
          routePositionRef.current,
          layer.speedRatio,
          reducedMotion && layer.name === "sky",
        );
        const style: TrainWorldLayerStyle = {
          "--train-layer-order": layerIndex,
          "--train-layer-position": `${initialLayerPosition.toFixed(3)}px`,
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
            data-layer-position={`${initialLayerPosition.toFixed(3)}px`}
            data-speed-ratio={layer.speedRatio}
            data-depth-saturation={depth.saturation}
            data-depth-brightness={depth.brightness}
            data-depth-contrast={depth.contrast}
            data-depth-scale={depth.scaleMultiplier}
            data-depth-detail-budget={depth.detailBudget}
            data-depth-anchor-tolerance={depth.anchorToContourTolerancePx}
            data-depth-overlap-limit={depth.maximumCollisionOverlapRatio}
            data-atmosphere-inside-sprite="false"
            data-motion={reducedMotion ? "reduced" : "full"}
            style={style}
            key={layer.name}
          >
            <div className="train-world-layer-track">
              {layerWindow.chunks.map((chunk) => (
                <TrainRouteChunk
                  chunk={chunk}
                  layer={layer}
                  includeSetPieces={false}
                  suppressScenery={projectedSetPieces[layer.name].some(
                    ({ focus }) =>
                      trainSetPieceReservationIntersectsChunk(
                        focus,
                        layer.speedRatio,
                        chunk.index,
                      ),
                  )}
                  key={`${layer.name}-${routeChunkSlotKey(
                    chunk.index,
                    layerWindow.chunks.length,
                  )}`}
                />
              ))}
              {projectedSetPieces[layer.name].map((projection) => (
                <TrainRouteChunk
                  chunk={projection.chunk}
                  layer={layer}
                  projection={projection}
                  key={
                    `${layer.name}:${projection.focus.id}:` +
                    `${projection.chunk.index}`
                  }
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
        data-haze-owner="dedicated-plane"
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
        data-haze-owner="dedicated-plane"
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
