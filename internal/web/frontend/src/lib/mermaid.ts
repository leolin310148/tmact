import { escapeAttribute } from "./dom";
import { flushFrontendLogs, logFrontend, type FrontendLogLevel } from "./frontendLog";

type MermaidAPI = typeof import("mermaid").default;

const MERMAID_LOAD_TIMEOUT_MS = 20_000;
const MERMAID_RENDER_TIMEOUT_MS = 15_000;

let mermaidInitialized = false;
let mermaidRenderSeq = 0;
let mermaidLoad: Promise<MermaidAPI> | null = null;
const verifyTimers = new WeakMap<HTMLElement, ReturnType<typeof setTimeout>>();
const renderedSVGBySource = new Map<string, string>();
const MAX_RENDERED_SVG_CACHE = 100;

function nowMS(): number {
  return typeof performance !== "undefined" && typeof performance.now === "function"
    ? performance.now()
    : Date.now();
}

function elapsedMS(start: number): number {
  return Math.round(nowMS() - start);
}

function sourcePreview(source: string): Record<string, unknown> {
  const trimmed = source.trim();
  return {
    source_len: source.length,
    source_head: trimmed.slice(0, 160),
    source_lines: trimmed ? trimmed.split(/\r?\n/).length : 0,
  };
}

function errorData(err: unknown): Record<string, unknown> {
  return err instanceof Error
    ? { error_name: err.name, error_message: err.message, error_stack: err.stack?.slice(0, 500) }
    : { error_message: String(err) };
}

function logMermaid(
  level: FrontendLogLevel,
  message: string,
  data: Record<string, unknown> = {},
): void {
  logFrontend(level, "mermaid_render", message, {
    ...data,
    visibility_state: typeof document !== "undefined" ? document.visibilityState : "unknown",
    online: typeof navigator !== "undefined" ? navigator.onLine : undefined,
  });
  void flushFrontendLogs({ keepalive: true });
}

async function loadMermaid(): Promise<MermaidAPI> {
  mermaidLoad ??= import("mermaid")
    .then((mod) => mod.default)
    .catch((err: unknown) => {
      mermaidLoad = null;
      throw err;
    });
  return mermaidLoad;
}

function timeoutError(message: string): Error {
  const err = new Error(message);
  err.name = "TimeoutError";
  return err;
}

function withTimeout<T>(promise: Promise<T>, ms: number, message: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | null = null;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(timeoutError(message)), ms);
  });
  return Promise.race([promise, timeout]).finally(() => {
    if (timer !== null) clearTimeout(timer);
  });
}

function initMermaid(mermaid: MermaidAPI): void {
  if (mermaidInitialized) return;
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict",
    theme: "dark",
    themeVariables: {
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
    },
  });
  mermaidInitialized = true;
}

function mermaidErrorNode(source: string, message: string): HTMLElement {
  const wrap = document.createElement("div");
  wrap.className = "markdown-preview-mermaid-error";

  const label = document.createElement("div");
  label.className = "markdown-preview-mermaid-error-label";
  label.textContent = message;

  const pre = document.createElement("pre");
  const code = document.createElement("code");
  code.className = "language-mermaid";
  code.textContent = source;
  pre.append(code);

  wrap.append(label, pre);
  return wrap;
}

function cacheRenderedSVG(source: string, svg: string): void {
  if (renderedSVGBySource.has(source)) renderedSVGBySource.delete(source);
  renderedSVGBySource.set(source, svg);
  while (renderedSVGBySource.size > MAX_RENDERED_SVG_CACHE) {
    const oldest = renderedSVGBySource.keys().next().value;
    if (oldest === undefined) break;
    renderedSVGBySource.delete(oldest);
  }
}

export function getCachedMermaidSVG(source: string): string | null {
  return renderedSVGBySource.get(source) ?? null;
}

export function renderCachedMermaidBlock(source: string, svg: string): string {
  return (
    '<div class="markdown-preview-mermaid" data-mermaid-state="rendered" data-mermaid-source="' +
    escapeAttribute(source) +
    '">' +
    svg +
    "</div>"
  );
}

function applyRenderedSVG(block: HTMLElement, source: string, svg: string): void {
  block.innerHTML = svg;
  block.dataset.mermaidSource = source;
  block.dataset.mermaidState = "rendered";
}

export function preserveRenderedMermaidBlocks(root: HTMLElement, html: string): string {
  const rendered = new Map<string, string>();
  for (const block of root.querySelectorAll<HTMLElement>(".markdown-preview-mermaid")) {
    const source = block.getAttribute("data-mermaid-source");
    if (!source || block.dataset.mermaidState !== "rendered" || !block.querySelector("svg")) continue;
    rendered.set(source, block.outerHTML);
  }
  if (rendered.size === 0) return html;

  return html.replace(
    /<div class="markdown-preview-mermaid" data-mermaid-state="pending" data-mermaid-source="([^"]*)"><div class="markdown-preview-mermaid-loading">Rendering diagram\.\.\.<\/div><\/div>/g,
    (placeholder, source: string) => rendered.get(source) ?? placeholder,
  );
}

function schedulePostRenderVerify(root: HTMLElement, runID: string): void {
  const existing = verifyTimers.get(root);
  if (existing) clearTimeout(existing);
  const timer = setTimeout(() => {
    verifyTimers.delete(root);
    if (!root.isConnected) {
      logMermaid("warn", "post verify skipped detached root", { run_id: runID });
      return;
    }

    const blocks = Array.from(root.querySelectorAll<HTMLElement>(".markdown-preview-mermaid"));
    const states = blocks.map((block, index) => ({
      index,
      state: block.dataset.mermaidState || "missing",
      has_svg: block.querySelector("svg") !== null,
      has_loading: block.querySelector(".markdown-preview-mermaid-loading") !== null,
      child_count: block.childElementCount,
    }));
    const needsRetry = states.some((state) => state.state !== "rendered" || !state.has_svg);
    logMermaid(needsRetry ? "warn" : "info", "post verify", {
      run_id: runID,
      blocks: blocks.length,
      states,
    });
    if (needsRetry) void renderMermaidDiagrams(root);
  }, 500);
  verifyTimers.set(root, timer);
}

export async function renderMermaidDiagrams(root: HTMLElement): Promise<void> {
  const runID = `mermaid-run-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
  const blocks = Array.from(root.querySelectorAll<HTMLElement>(".markdown-preview-mermaid"));
  if (blocks.length === 0) {
    logMermaid("info", "scan empty", { run_id: runID });
    return;
  }
  logMermaid("info", "scan blocks", {
    run_id: runID,
    blocks: blocks.length,
    states: blocks.map((block) => block.dataset.mermaidState || "missing"),
  });

  const pending = blocks.filter((block) => {
    const state = block.dataset.mermaidState;
    return state !== "rendered" && state !== "rendering" && state !== "error";
  });
  if (pending.length === 0) {
    logMermaid("info", "no pending blocks", {
      run_id: runID,
      states: blocks.map((block) => block.dataset.mermaidState || "missing"),
    });
    return;
  }

  const sources = new Map<HTMLElement, string>();
  let cachedCount = 0;
  for (const [idx, block] of pending.entries()) {
    const source = block.dataset.mermaidSource ?? block.textContent ?? "";
    if (!source.trim()) continue;
    const cachedSVG = renderedSVGBySource.get(source);
    if (cachedSVG) {
      applyRenderedSVG(block, source, cachedSVG);
      cachedCount++;
      logMermaid("info", "cache hit", {
        run_id: runID,
        block_index: idx,
        svg_len: cachedSVG.length,
        ...sourcePreview(source),
      });
      continue;
    }
    sources.set(block, source);
    block.dataset.mermaidState = "rendering";
    logMermaid("info", "block queued", {
      run_id: runID,
      block_index: idx,
      ...sourcePreview(source),
    });
  }
  if (sources.size === 0) {
    if (cachedCount > 0) {
      logMermaid("info", "run complete from cache", {
        run_id: runID,
        blocks: cachedCount,
      });
      schedulePostRenderVerify(root, runID);
      return;
    }
    logMermaid("warn", "no non-empty sources", { run_id: runID, pending: pending.length });
    return;
  }

  let mermaid: MermaidAPI;
  const loadStart = nowMS();
  try {
    logMermaid("info", "load start", { run_id: runID, timeout_ms: MERMAID_LOAD_TIMEOUT_MS });
    mermaid = await withTimeout(
      loadMermaid(),
      MERMAID_LOAD_TIMEOUT_MS,
      `Timed out loading Mermaid renderer after ${MERMAID_LOAD_TIMEOUT_MS / 1000}s`,
    );
    initMermaid(mermaid);
    logMermaid("info", "load ok", { run_id: runID, elapsed_ms: elapsedMS(loadStart) });
  } catch (err) {
    if (err instanceof Error && err.name === "TimeoutError") mermaidLoad = null;
    const message = err instanceof Error ? err.message : "Unable to load Mermaid renderer";
    logMermaid("error", "load failed", {
      run_id: runID,
      elapsed_ms: elapsedMS(loadStart),
      ...errorData(err),
    });
    for (const [block, source] of sources) {
      block.replaceChildren(mermaidErrorNode(source, message));
      block.dataset.mermaidState = "error";
    }
    return;
  }

  let blockIndex = 0;
  for (const [block, source] of sources) {
    const renderStart = nowMS();
    const id = `tmact-mermaid-${++mermaidRenderSeq}`;
    try {
      logMermaid("info", "render start", {
        run_id: runID,
        block_index: blockIndex,
        render_id: id,
        timeout_ms: MERMAID_RENDER_TIMEOUT_MS,
        ...sourcePreview(source),
      });
      const { svg, bindFunctions } = await withTimeout(
        mermaid.render(id, source),
        MERMAID_RENDER_TIMEOUT_MS,
        `Timed out rendering Mermaid diagram after ${MERMAID_RENDER_TIMEOUT_MS / 1000}s`,
      );
      if (!root.contains(block)) {
        logMermaid("warn", "render result skipped detached block", {
          run_id: runID,
          block_index: blockIndex,
          render_id: id,
          elapsed_ms: elapsedMS(renderStart),
          ...sourcePreview(source),
        });
        blockIndex++;
        continue;
      }
      cacheRenderedSVG(source, svg);
      applyRenderedSVG(block, source, svg);
      bindFunctions?.(block);
      logMermaid("info", "render ok", {
        run_id: runID,
        block_index: blockIndex,
        render_id: id,
        elapsed_ms: elapsedMS(renderStart),
        svg_len: svg.length,
        dom_state: block.dataset.mermaidState,
        has_svg: block.querySelector("svg") !== null,
        is_connected: block.isConnected,
        child_count: block.childElementCount,
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unable to render Mermaid diagram";
      logMermaid("error", "render failed", {
        run_id: runID,
        block_index: blockIndex,
        render_id: id,
        elapsed_ms: elapsedMS(renderStart),
        ...errorData(err),
      });
      block.replaceChildren(mermaidErrorNode(source, message));
      block.dataset.mermaidState = "error";
    }
    blockIndex++;
  }
  logMermaid("info", "run complete", { run_id: runID, blocks: sources.size });
  schedulePostRenderVerify(root, runID);
}
