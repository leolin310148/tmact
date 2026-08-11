export type SceneMode = "day" | "sunset" | "night";

export const SCENE_MODES: readonly SceneMode[] = ["day", "sunset", "night"];

const SCENE_MODE_BOUNDARIES = [
  [6, 0],
  [17, 0],
  [18, 30],
] as const;

// The office layout uses these stable local-time boundaries: day from 06:00,
// sunset from 17:00, and night from 18:30.
export function clockSceneMode(now: Date): SceneMode {
  const minutes = now.getHours() * 60 + now.getMinutes();
  if (minutes >= 17 * 60 && minutes < 18 * 60 + 30) return "sunset";
  if (minutes >= 6 * 60 && minutes < 17 * 60) return "day";
  return "night";
}

export function nextClockSceneModeBoundary(now: Date): Date {
  for (const [hours, minutes] of SCENE_MODE_BOUNDARIES) {
    const boundary = new Date(now);
    boundary.setHours(hours, minutes, 0, 0);
    if (boundary.getTime() > now.getTime()) return boundary;
  }

  const nextDay = new Date(now);
  nextDay.setDate(nextDay.getDate() + 1);
  nextDay.setHours(
    SCENE_MODE_BOUNDARIES[0][0],
    SCENE_MODE_BOUNDARIES[0][1],
    0,
    0,
  );
  return nextDay;
}

export function nextSceneMode(mode: SceneMode): SceneMode {
  return SCENE_MODES[(SCENE_MODES.indexOf(mode) + 1) % SCENE_MODES.length]!;
}
