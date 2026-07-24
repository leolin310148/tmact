export type SceneMode = "day" | "sunset" | "night";

export const SCENE_MODES: readonly SceneMode[] = ["day", "sunset", "night"];

// Shared by the office and train layouts so both local-time scenes cross the
// same boundaries: day from 06:00, sunset from 17:00, and night from 18:30.
export function clockSceneMode(now: Date): SceneMode {
  const minutes = now.getHours() * 60 + now.getMinutes();
  if (minutes >= 17 * 60 && minutes < 18 * 60 + 30) return "sunset";
  if (minutes >= 6 * 60 && minutes < 17 * 60) return "day";
  return "night";
}

export function nextSceneMode(mode: SceneMode): SceneMode {
  return SCENE_MODES[(SCENE_MODES.indexOf(mode) + 1) % SCENE_MODES.length]!;
}
