// Pure terminal renderer ported 1:1 from static/js/terminal.js.
//
// The original `terminal.js` exposed a single imperative `setContent(text, opts)`
// that built an HTML string and then assigned it to `pre#content.innerHTML`,
// followed by a DOM-mutating `markImagePaths` pass and an auto-scroll. For the
// React port the HTML-building half is split out as a PURE function `render`
// (no DOM, no React) so it can be unit-tested as a string; the DOM-mutating
// `markImagePaths` stays a separate exported function that ContentPane runs in a
// layout effect after assigning the rendered HTML. The auto-scroll lives in
// ContentPane too (see ARCHITECTURE §7). Behavior is byte-for-behavior with the
// original — same palette, same regexes, same private-use-area placeholder
// ordering (rules → tables → URLs), same class names (tui-rule / tui-link /
// image-path / tui-table*).

import { escapeHTML } from "../lib/dom";

// Tango palette: the 16 base colours, readable on the dark background.
const ANSI16: readonly string[] = [
  "#000000", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
  "#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec",
];

export function ansi256(n: number): string {
  if (n < 16) return ANSI16[n] ?? "";
  if (n < 232) {
    n -= 16;
    const hex = (v: number): string => (v === 0 ? 0 : 55 + v * 40).toString(16).padStart(2, "0");
    return "#" + hex(Math.floor(n / 36) % 6) + hex(Math.floor(n / 6) % 6) + hex(n % 6);
  }
  const g = (8 + (n - 232) * 10).toString(16).padStart(2, "0");
  return "#" + g + g + g;
}

interface SgrState {
  fg: string | null;
  bg: string | null;
  bold: boolean;
  dim: boolean;
  italic: boolean;
  underline: boolean;
  reverse: boolean;
}

// ansiToHTML turns tmux -e output (plain text + \x1b[...m SGR sequences) into
// HTML. Text is HTML-escaped; styles come only from a fixed palette and parsed
// integers, never from raw pane text, so a coloured span cannot inject markup.
export function ansiToHTML(raw: string): string {
  const st: SgrState = {
    fg: null, bg: null, bold: false, dim: false,
    italic: false, underline: false, reverse: false,
  };
  const reset = (): void => {
    st.fg = st.bg = null;
    st.bold = st.dim = st.italic = st.underline = st.reverse = false;
  };
  const apply = (codes: number[]): void => {
    for (let i = 0; i < codes.length; i++) {
      const c = codes[i];
      if (c === 0) reset();
      else if (c === 1) st.bold = true;
      else if (c === 2) st.dim = true;
      else if (c === 3) st.italic = true;
      else if (c === 4) st.underline = true;
      else if (c === 7) st.reverse = true;
      else if (c === 22) st.bold = st.dim = false;
      else if (c === 23) st.italic = false;
      else if (c === 24) st.underline = false;
      else if (c === 27) st.reverse = false;
      else if (c !== undefined && c >= 30 && c <= 37) st.fg = ANSI16[c - 30] ?? null;
      else if (c === 39) st.fg = null;
      else if (c !== undefined && c >= 40 && c <= 47) st.bg = ANSI16[c - 40] ?? null;
      else if (c === 49) st.bg = null;
      else if (c !== undefined && c >= 90 && c <= 97) st.fg = ANSI16[c - 82] ?? null;
      else if (c !== undefined && c >= 100 && c <= 107) st.bg = ANSI16[c - 92] ?? null;
      else if (c === 38 || c === 48) {
        const key = c === 38 ? "fg" : "bg";
        if (codes[i + 1] === 5) { st[key] = ansi256((codes[i + 2] ?? 0) | 0); i += 2; }
        else if (codes[i + 1] === 2) {
          const ch = (v: number): number => Math.max(0, Math.min(255, v | 0));
          st[key] = "rgb(" + ch(codes[i + 2] ?? 0) + "," + ch(codes[i + 3] ?? 0) + "," + ch(codes[i + 4] ?? 0) + ")";
          i += 4;
        }
      }
    }
  };
  const style = (): string => {
    let f = st.fg, b = st.bg;
    if (st.reverse) { f = st.bg || "var(--bg)"; b = st.fg || "var(--fg)"; }
    const p: string[] = [];
    if (f) p.push("color:" + f);
    if (b) p.push("background:" + b);
    if (st.bold) p.push("font-weight:700");
    if (st.dim) p.push("opacity:.6");
    if (st.italic) p.push("font-style:italic");
    if (st.underline) p.push("text-decoration:underline");
    return p.join(";");
  };

  let out = "", last = 0;
  let m: RegExpExecArray | null;
  // First alternative is SGR (parsed); the rest are other CSI/OSC escapes (dropped).
  const re = /\x1b\[([0-9;]*)m|\x1b\[[0-9;?]*[ -\/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
  const emit = (from: number, to: number): void => {
    if (to <= from) return;
    const seg = escapeHTML(raw.slice(from, to));
    const s = style();
    out += s ? '<span style="' + s + '">' + seg + "</span>" : seg;
  };
  while ((m = re.exec(raw)) !== null) {
    emit(last, m.index);
    if (m[1] !== undefined) {
      apply(m[1] === "" ? [0] : m[1].split(";").map((n) => parseInt(n, 10) || 0));
    }
    last = re.lastIndex;
  }
  emit(last, raw.length);
  return out;
}

// wrapRuleLines replaces separator lines (rows of U+2500 or ASCII hyphens)
// with private-use markers that survive HTML escaping. render() then converts
// the markers to a CSS-drawn rule, so the original terminal-width run of rule
// characters never wraps or leaks into the rendered pane.
const RULE_OPEN = "", RULE_CLOSE = "";
// extractTables collapses box-drawing tables (┌─┐ / │…│ / └─┘) into a single
// PUA placeholder line so the surrounding pre-wrap layout never tries to align
// columns at terminal cell widths — a fight the web font always loses. The
// real HTML <table> is spliced back in after ansiToHTML.
const TABLE_OPEN = "", TABLE_CLOSE = "";
const TABLE_PLACEHOLDER_RE = /(\d+)/g;
// extractURLs wraps URL spans so a long-press "Copy link" on mobile yields a
// clean URL even when terminal wrap split the URL across lines with leading
// indent. URL_OPEN<idx>URL_SEP<visible>URL_CLOSE markers survive ansiToHTML;
// the visible body keeps its original \n + spaces inside the rendered <a> so
// the pane stays visually faithful to the terminal layout.
const URL_OPEN = "", URL_SEP = "", URL_CLOSE = "";
const URL_PLACEHOLDER_RE = /(\d+)([\s\S]*?)/g;
// URL detection. The char class is RFC 3986 reserved + unreserved + percent —
// ASCII only, so CJK punctuation, backslashes, and prose can't be glued onto a
// URL. ANSI escape sequences are allowed mid-URL: tmux `capture-pane -e -J`
// re-asserts SGR state at a soft-wrap join, so a visually contiguous URL like
// github.com/x/y is stored as `github.com/x\x1b[...m/y` in raw output. The
// match extends across a soft wrap only when the next line starts with
// horizontal whitespace AND a URL-path character, covering the github-link
// wrap without joining prose onto URLs.
const URL_CHARS = "A-Za-z0-9\\-._~:/?#\\[\\]@!$&*+,;=%()";
const URL_ANSI = "(?:\\x1b\\[[0-9;?]*[ -/]*[@-~]|\\x1b\\][^\\x07\\x1b]*(?:\\x07|\\x1b\\\\))";
const URL_ATOM = "(?:[" + URL_CHARS + "]|" + URL_ANSI + ")";
const URL_RE = new RegExp(
  "https?://" + URL_ATOM + "+(?:\\n[ \\t]+(?:[/?&=#+%~@:.\\-]|" + URL_ANSI + ")" + URL_ATOM + "*)*",
  "g",
);
const URL_TRAIL_RE = /[.,;:!?)\]}>'"`]+$/;
const URL_ANSI_RE = /\x1b\[[0-9;?]*[ -\/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
const ANSI_STRIP_RE = /\x1b\[[0-9;?]*[ -\/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
export const IMAGE_PATH_RE = /(?:file:\/\/)?(?:~\/|\.{1,2}\/|\/)?[A-Za-z0-9_./~:@%+,-][^\s"'`<>]*\.(?:png|jpe?g|gif|webp|bmp|svg)(?=$|[\s"'`<>)\]}.,;:!?])/gi;
const URL_SCHEME_RE = /^[A-Za-z][A-Za-z0-9+.-]*:\/\//;

export function previewableImagePath(path: string): boolean {
  if (path.startsWith("~/")) return false;
  const scheme = URL_SCHEME_RE.exec(path);
  return !scheme || scheme[0].toLowerCase() === "file://";
}

// Box-drawing borders we treat as table frames. The top/bottom-corner regexes
// only match a line whose visible characters are entirely frame chars, so a
// `┌` that happens to appear mid-line in pane output (e.g. inside agent text)
// will not start a false-positive table.
const TABLE_TOP_RE = /^[ \t]*[┌╔][─═]+(?:[┬╦][─═]+)*[┐╗][ \t]*$/;
const TABLE_BOT_RE = /^[ \t]*[└╚][─═]+(?:[┴╩][─═]+)*[┘╝][ \t]*$/;
const TABLE_SEP_RE = /^[ \t]*[├╠][─═]+(?:[┼╬][─═]+)*[┤╣][ \t]*$/;
// A line that's *only* mid/end frame chars (no start corner) is a wrap
// continuation of the preceding frame line. joinWrappedFrames merges those
// back so the per-line top/bot/sep regexes can match the full frame.
const FRAME_START_RE = /^[ \t]*[┌├└╔╠╚]/;
const FRAME_CONT_RE = /^[ \t]*[─═┬┴┼╦╩╬][─═┬┴┼╦╩╬┐┘┤╗╝╣ \t]*[┐┘┤╗╝╣─═┬┴┼╦╩╬][ \t]*$/;

export function joinWrappedFrames(lines: string[]): string[] {
  const out: string[] = [];
  for (const raw of lines) {
    if (out.length > 0) {
      const v = raw.replace(ANSI_STRIP_RE, "");
      const prev = out[out.length - 1] ?? "";
      const prevV = prev.replace(ANSI_STRIP_RE, "");
      const prevIsFrame =
        FRAME_START_RE.test(prevV) ||
        (FRAME_CONT_RE.test(prevV) && !/[┐┘┤╗╝╣]\s*$/.test(prevV));
      if (prevIsFrame && FRAME_CONT_RE.test(v)) {
        out[out.length - 1] = prev + v.replace(/^[ \t]+/, "");
        continue;
      }
    }
    out.push(raw);
  }
  return out;
}

export interface ParsedTable {
  rows: string[][];
  headerEnd: number;
}

// parseTableBlock joins terminal-wrapped continuation lines into logical rows.
// The column count comes from the top frame's ┬/╦ markers; each row segment
// (delimited by ├─┤ or the top/bottom frame) is concatenated then split by
// │/║. A row whose cell count doesn't match `cols` invalidates the whole
// block — better to fall back to raw text than render a misaligned table.
export function parseTableBlock(blockLines: string[]): ParsedTable {
  let cols = 0;
  for (const raw of blockLines) {
    const v = raw.replace(ANSI_STRIP_RE, "");
    if (TABLE_TOP_RE.test(v)) {
      const m = v.match(/[┬╦]/g);
      cols = (m ? m.length : 0) + 1;
      break;
    }
  }
  if (cols < 1) return { rows: [], headerEnd: -1 };

  const rows: string[][] = [];
  let headerEnd = -1;
  let buf = "";
  let invalid = false;

  const flush = (): void => {
    if (!buf) return;
    // Strip leading/trailing edge bars (and the surrounding whitespace that
    // terminal padding may have inserted), then split on inner bars.
    const trimmed = buf.replace(/^[ \t]*[│║]/, "").replace(/[│║][ \t]*$/, "");
    const cells = trimmed.split(/[│║]/).map((c) => c.trim());
    if (cells.length !== cols) { invalid = true; }
    else rows.push(cells);
    buf = "";
  };

  for (const raw of blockLines) {
    const v = raw.replace(ANSI_STRIP_RE, "");
    if (TABLE_TOP_RE.test(v) || TABLE_BOT_RE.test(v)) { flush(); continue; }
    if (TABLE_SEP_RE.test(v)) {
      flush();
      if (!invalid && headerEnd === -1 && rows.length > 0) headerEnd = rows.length;
      continue;
    }
    // A continuation line may have no bars (wrap mid-cell). Always append; a
    // complete row is signalled by accumulating exactly cols+1 bars total —
    // anything beyond that means we glued two rows together and need to flush.
    buf += v;
    const bars = (buf.match(/[│║]/g) || []).length;
    if (bars >= cols + 1) flush();
  }
  flush();

  if (invalid) return { rows: [], headerEnd: -1 };
  return { rows, headerEnd };
}

export function renderTable(parsed: ParsedTable): string {
  const { rows, headerEnd } = parsed;
  if (!rows.length) return "";
  const hEnd = headerEnd > 0 ? headerEnd : 0;
  const parts: string[] = ['<div class="tui-table-wrap"><table class="tui-table">'];
  if (hEnd > 0) {
    parts.push("<thead>");
    for (let r = 0; r < hEnd; r++) {
      parts.push("<tr>");
      for (const c of rows[r] ?? []) parts.push("<th>" + escapeHTML(c) + "</th>");
      parts.push("</tr>");
    }
    parts.push("</thead>");
  }
  parts.push("<tbody>");
  for (let r = hEnd; r < rows.length; r++) {
    parts.push("<tr>");
    for (const c of rows[r] ?? []) parts.push("<td>" + escapeHTML(c) + "</td>");
    parts.push("</tr>");
  }
  parts.push("</tbody></table></div>");
  return parts.join("");
}

export interface ExtractedTables {
  text: string;
  tables: string[];
}

export function extractTables(text: string): ExtractedTables {
  const lines = joinWrappedFrames(text.split("\n"));
  const tables: string[] = [];
  const out: string[] = [];
  let i = 0;
  while (i < lines.length) {
    const cur = lines[i] ?? "";
    const v = cur.replace(ANSI_STRIP_RE, "");
    if (TABLE_TOP_RE.test(v)) {
      // Scan ahead for the bottom frame. Tolerate intervening lines without
      // bars: terminal wrap can split a row into a frame-less continuation
      // line. We cap the scan so a stray ┌─┐ in prose without a closer can't
      // swallow the rest of the buffer.
      let j = i + 1;
      const limit = Math.min(lines.length, i + 1 + 600);
      while (j < limit) {
        const vj = (lines[j] ?? "").replace(ANSI_STRIP_RE, "");
        if (TABLE_BOT_RE.test(vj)) break;
        // Bail if we hit another top frame before closing — broken nesting.
        if (TABLE_TOP_RE.test(vj)) { j = -1; break; }
        j++;
      }
      if (j > 0 && j < lines.length && TABLE_BOT_RE.test((lines[j] ?? "").replace(ANSI_STRIP_RE, ""))) {
        const parsed = parseTableBlock(lines.slice(i, j + 1));
        if (parsed.rows.length > 0) {
          const idx = tables.length;
          tables.push(renderTable(parsed));
          out.push(TABLE_OPEN + idx + TABLE_CLOSE);
          i = j + 1;
          continue;
        }
      }
    }
    out.push(cur);
    i++;
  }
  return { text: out.join("\n"), tables };
}

export interface ExtractedURLs {
  text: string;
  urls: string[];
}

export function extractURLs(text: string): ExtractedURLs {
  const urls: string[] = [];
  const replaced = text.replace(URL_RE, (match) => {
    // Pull ANSI escapes out of the body and re-emit them AFTER the placeholder
    // so the SGR state transitions still apply to subsequent text, but the
    // body itself is a flat string — keeping ansiToHTML from crossing <a> and
    // <span> boundaries (which produces malformed HTML).
    const trailingAnsi = (match.match(URL_ANSI_RE) || []).join("");
    let clean = match.replace(URL_ANSI_RE, "");
    let trailing = "";
    const tm = clean.match(URL_TRAIL_RE);
    if (tm && tm[0] !== undefined) { trailing = tm[0]; clean = clean.slice(0, -trailing.length); }
    if (!/^https?:\/\/.+/.test(clean)) return match;
    const href = clean.replace(/\n[ \t]+/g, "");
    const idx = urls.length;
    urls.push(href);
    return URL_OPEN + idx + URL_SEP + clean + URL_CLOSE + trailing + trailingAnsi;
  });
  return { text: replaced, urls };
}

export function wrapRuleLines(text: string): string {
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const visible = (lines[i] ?? "").replace(ANSI_STRIP_RE, "");
    const ruleChars = visible.replace(/\s/g, "");
    if (!/^[─-]+$/.test(ruleChars)) continue;
    if (ruleChars.length < 8) continue;
    lines[i] = RULE_OPEN + RULE_CLOSE;
  }
  return lines.join("\n");
}

export interface RenderOpts {
  cwd?: string;
  peer?: string;
}

// render is the PURE half of the original setContent: it produces the final HTML
// string that the caller assigns to pre#content.innerHTML. The placeholder
// replacement order is exactly the original's: rules → tables → URLs (the
// private-use markers all survive escapeHTML). `opts` is accepted for API
// symmetry with the original setContent(text, opts), but the HTML render does
// NOT depend on cwd/peer — those are only used by markImagePaths, which runs as
// a separate DOM pass after the HTML is in the document (see ContentPane).
export function render(text: string, _opts?: RenderOpts): string {
  void _opts;
  const extracted = extractTables(text);
  const linkified = extractURLs(extracted.text);
  let html = ansiToHTML(wrapRuleLines(linkified.text))
    .replaceAll(RULE_OPEN, '<span class="tui-rule" role="separator">')
    .replaceAll(RULE_CLOSE, "</span>");
  if (extracted.tables.length) {
    html = html.replace(TABLE_PLACEHOLDER_RE, (_, n: string) => extracted.tables[+n] || "");
  }
  if (linkified.urls.length) {
    html = html.replace(URL_PLACEHOLDER_RE, (_, n: string, body: string) => {
      const href = linkified.urls[+n];
      if (!href) return body;
      // target=_blank gives desktop click-to-open; long-press on mobile copies
      // the clean href (without the terminal-wrap newline/indent in `body`).
      return '<a href="' + escapeHTML(href) + '" target="_blank" rel="noopener noreferrer" class="tui-link">' + body + "</a>";
    });
  }
  return html;
}

// markImagePaths is the DOM-mutating half of the original setContent. It walks
// every text node under `root`, wraps each previewable image path in a
// <span class="image-path" data-path data-cwd data-peer>, and leaves everything
// else untouched. ContentPane runs this in a useLayoutEffect AFTER assigning
// render()'s HTML to pre#content, so React never reconciles the inner HTML.
//
// NB: matches the original ordering — all text nodes are collected first (so
// replacing one node's content can't perturb the live TreeWalker), then each is
// rewritten in a second pass.
export function markImagePaths(root: Node, cwd?: string | null, peer?: string | null): void {
  const doc = root.ownerDocument ?? document;
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const nodes: Text[] = [];
  while (walker.nextNode()) nodes.push(walker.currentNode as Text);

  for (const node of nodes) {
    const text = node.nodeValue ?? "";
    IMAGE_PATH_RE.lastIndex = 0;
    let m: RegExpExecArray | null;
    let last = 0;
    const frag = doc.createDocumentFragment();
    while ((m = IMAGE_PATH_RE.exec(text)) !== null) {
      const match = m[0]; // group 0 of a successful exec is always present
      if (match === undefined || !previewableImagePath(match)) continue;
      if (m.index > last) frag.appendChild(doc.createTextNode(text.slice(last, m.index)));
      const span = doc.createElement("span");
      span.className = "image-path";
      span.textContent = match;
      span.dataset.path = match;
      if (cwd) span.dataset.cwd = cwd;
      if (peer) span.dataset.peer = peer;
      span.title = "Command-click to preview image";
      frag.appendChild(span);
      last = IMAGE_PATH_RE.lastIndex;
    }
    if (last === 0) continue;
    if (last < text.length) frag.appendChild(doc.createTextNode(text.slice(last)));
    node.parentNode?.replaceChild(frag, node);
  }
}

// __test__ mirrors the original terminal.js export so the ported unit tests can
// reach the internal helpers by the same names.
export const __test__ = { extractTables, parseTableBlock, renderTable, joinWrappedFrames, extractURLs };
