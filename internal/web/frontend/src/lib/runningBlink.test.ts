import { describe, expect, it } from "vitest";

import { RUNNING_PHASE_MS, runningPhaseAt, startRunningBlink } from "./runningBlink";

function fakeEnv(start: number) {
  let now = start;
  let pending: { fn: () => void; at: number } | null = null;
  const listeners: Record<string, () => void> = {};
  const mq = {
    matches: false,
    addEventListener: (_: string, fn: () => void) => (listeners.motion = fn),
    removeEventListener: () => delete listeners.motion,
  };
  const doc = {
    hidden: false,
    addEventListener: (_: string, fn: () => void) => (listeners.vis = fn),
    removeEventListener: () => delete listeners.vis,
  };
  const root = document.createElement("html");
  const env = {
    root,
    doc: doc as unknown as Document,
    now: () => now,
    setTimeout: (fn: () => void, ms: number) => {
      pending = { fn, at: now + ms };
      return pending;
    },
    clearTimeout: (id: unknown) => {
      if (pending === id) pending = null;
    },
    reducedMotion: mq as unknown as MediaQueryList,
  };
  const advance = () => {
    const p = pending!;
    now = p.at;
    pending = null;
    p.fn();
  };
  return { env, root, doc, mq, listeners, advance, pendingAt: () => pending?.at ?? null };
}

describe("running blink clock", () => {
  it("steps phases on wall-clock boundaries so split slots flip together", () => {
    const f = fakeEnv(RUNNING_PHASE_MS * 5 + 123);
    const stop = startRunningBlink(f.env);
    expect(f.root.dataset.runningPhase).toBe(String(runningPhaseAt(RUNNING_PHASE_MS * 5)));
    expect(f.pendingAt()).toBe(RUNNING_PHASE_MS * 6);
    f.advance();
    expect(f.root.dataset.runningPhase).toBe("2");
    f.advance();
    expect(f.root.dataset.runningPhase).toBe("3");
    f.advance();
    expect(f.root.dataset.runningPhase).toBe("0");
    stop();
    expect(f.root.dataset.runningPhase).toBeUndefined();
    expect(f.pendingAt()).toBeNull();
  });

  it("idles while hidden and resumes when visible", () => {
    const f = fakeEnv(0);
    startRunningBlink(f.env);
    f.doc.hidden = true;
    f.listeners.vis!();
    expect(f.root.dataset.runningPhase).toBeUndefined();
    expect(f.pendingAt()).toBeNull();
    f.doc.hidden = false;
    f.listeners.vis!();
    expect(f.root.dataset.runningPhase).toBe("0");
    expect(f.pendingAt()).toBe(RUNNING_PHASE_MS);
  });

  it("never steps under reduced motion", () => {
    const f = fakeEnv(0);
    f.mq.matches = true;
    startRunningBlink(f.env);
    expect(f.root.dataset.runningPhase).toBeUndefined();
    expect(f.pendingAt()).toBeNull();
    f.mq.matches = false;
    f.listeners.motion!();
    expect(f.root.dataset.runningPhase).toBe("0");
  });
});
