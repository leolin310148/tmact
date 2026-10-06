// Low-frequency stepped "animation" clock for the running-agent effects.
//
// The running effects used to be infinite CSS @keyframes. Even when they only
// animate compositor-friendly properties (opacity/filter/transform), an
// infinite animation makes Chrome produce a new frame every vsync (120Hz on a
// ProMotion MacBook) and WindowServer recomposite the window each time — in
// split view that is 3 iframes × every running icon. Measured on a MacBook
// with three split slots: Chrome GPU ~31% → ~2% and renderer ~19% → ~8% once
// the effect stopped animating.
//
// Instead, one timer per document steps a phase attribute on <html>
// (data-running-phase = 0..3) every PHASE_MS, and app.css maps each phase to a
// static look with no transition. That costs one style recalc + one frame per
// step instead of ~170 frames per cycle. Steps are aligned to wall-clock
// multiples of PHASE_MS so the split slots (separate documents, same clock)
// flip together and share their frames.

export const RUNNING_PHASE_MS = 700;
export const RUNNING_PHASES = 4;

export interface RunningBlinkEnv {
  root: HTMLElement;
  doc: Pick<Document, "hidden" | "addEventListener" | "removeEventListener">;
  now: () => number;
  setTimeout: (fn: () => void, ms: number) => unknown;
  clearTimeout: (id: unknown) => void;
  reducedMotion: MediaQueryList | null;
}

export function runningPhaseAt(nowMs: number): number {
  return Math.floor(nowMs / RUNNING_PHASE_MS) % RUNNING_PHASES;
}

// startRunningBlink starts the phase clock and returns a stop function. The
// clock idles (attribute removed, no timer) while the document is hidden or
// the user prefers reduced motion; app.css then shows the static look.
export function startRunningBlink(env?: Partial<RunningBlinkEnv>): () => void {
  const e: RunningBlinkEnv = {
    root: env?.root ?? document.documentElement,
    doc: env?.doc ?? document,
    now: env?.now ?? (() => Date.now()),
    setTimeout: env?.setTimeout ?? ((fn, ms) => window.setTimeout(fn, ms)),
    clearTimeout: env?.clearTimeout ?? ((id) => window.clearTimeout(id as number)),
    reducedMotion:
      env?.reducedMotion !== undefined
        ? env.reducedMotion
        : typeof window.matchMedia === "function"
          ? window.matchMedia("(prefers-reduced-motion: reduce)")
          : null,
  };

  let timer: unknown = null;

  const halt = () => {
    if (timer != null) e.clearTimeout(timer);
    timer = null;
    delete e.root.dataset.runningPhase;
  };

  const tick = () => {
    const t = e.now();
    e.root.dataset.runningPhase = String(runningPhaseAt(t));
    timer = e.setTimeout(tick, RUNNING_PHASE_MS - (t % RUNNING_PHASE_MS));
  };

  const sync = () => {
    if (e.doc.hidden || e.reducedMotion?.matches) {
      halt();
    } else if (timer == null) {
      tick();
    }
  };

  e.doc.addEventListener("visibilitychange", sync);
  e.reducedMotion?.addEventListener?.("change", sync);
  sync();

  return () => {
    e.doc.removeEventListener("visibilitychange", sync);
    e.reducedMotion?.removeEventListener?.("change", sync);
    halt();
  };
}
