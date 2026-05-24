import { $, escapeHTML } from "./dom.js";

// Tango palette: the 16 base colours, readable on the dark background.
const ANSI16 = [
  "#000000", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
  "#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec",
];

function ansi256(n) {
  if (n < 16) return ANSI16[n] || "";
  if (n < 232) {
    n -= 16;
    const hex = (v) => (v === 0 ? 0 : 55 + v * 40).toString(16).padStart(2, "0");
    return "#" + hex(Math.floor(n / 36) % 6) + hex(Math.floor(n / 6) % 6) + hex(n % 6);
  }
  const g = (8 + (n - 232) * 10).toString(16).padStart(2, "0");
  return "#" + g + g + g;
}

// ansiToHTML turns tmux -e output (plain text + \x1b[...m SGR sequences) into
// HTML. Text is HTML-escaped; styles come only from a fixed palette and parsed
// integers, never from raw pane text, so a coloured span cannot inject markup.
function ansiToHTML(raw) {
  const st = { fg: null, bg: null, bold: false, dim: false,
               italic: false, underline: false, reverse: false };
  const reset = () => {
    st.fg = st.bg = null;
    st.bold = st.dim = st.italic = st.underline = st.reverse = false;
  };
  const apply = (codes) => {
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
      else if (c >= 30 && c <= 37) st.fg = ANSI16[c - 30];
      else if (c === 39) st.fg = null;
      else if (c >= 40 && c <= 47) st.bg = ANSI16[c - 40];
      else if (c === 49) st.bg = null;
      else if (c >= 90 && c <= 97) st.fg = ANSI16[c - 82];
      else if (c >= 100 && c <= 107) st.bg = ANSI16[c - 92];
      else if (c === 38 || c === 48) {
        const key = c === 38 ? "fg" : "bg";
        if (codes[i + 1] === 5) { st[key] = ansi256(codes[i + 2] | 0); i += 2; }
        else if (codes[i + 1] === 2) {
          const ch = (v) => Math.max(0, Math.min(255, v | 0));
          st[key] = "rgb(" + ch(codes[i+2]) + "," + ch(codes[i+3]) + "," + ch(codes[i+4]) + ")";
          i += 4;
        }
      }
    }
  };
  const style = () => {
    let f = st.fg, b = st.bg;
    if (st.reverse) { f = st.bg || "var(--bg)"; b = st.fg || "var(--fg)"; }
    const p = [];
    if (f) p.push("color:" + f);
    if (b) p.push("background:" + b);
    if (st.bold) p.push("font-weight:700");
    if (st.dim) p.push("opacity:.6");
    if (st.italic) p.push("font-style:italic");
    if (st.underline) p.push("text-decoration:underline");
    return p.join(";");
  };

  let out = "", last = 0, m;
  // First alternative is SGR (parsed); the rest are other CSI/OSC escapes (dropped).
  const re = /\x1b\[([0-9;]*)m|\x1b\[[0-9;?]*[ -\/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
  const emit = (from, to) => {
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
// with private-use markers that survive HTML escaping. setContent then converts
// the markers to a CSS-drawn rule, so the original terminal-width run of rule
// characters never wraps or leaks into the rendered pane.
const RULE_OPEN = "", RULE_CLOSE = "";
// extractTables collapses box-drawing tables (┌─┐ / │…│ / └─┘) into a single
// PUA placeholder line so the surrounding pre-wrap layout never tries to align
// columns at terminal cell widths — a fight the web font always loses. The
// real HTML <table> is spliced back in after ansiToHTML.
const TABLE_OPEN = "", TABLE_CLOSE = "";
const TABLE_PLACEHOLDER_RE = /(\d+)/g;
const ANSI_STRIP_RE = /\x1b\[[0-9;?]*[ -\/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
const IMAGE_PATH_RE = /(?:file:\/\/)?(?:~\/|\.{1,2}\/|\/)?[A-Za-z0-9_./~:@%+,-][^\s"'`<>]*\.(?:png|jpe?g|gif|webp|bmp|svg)(?=$|[\s"'`<>)\]}.,;:!?])/gi;
const URL_SCHEME_RE = /^[A-Za-z][A-Za-z0-9+.-]*:\/\//;

function previewableImagePath(path) {
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
const TABLE_ROW_RE = /^[ \t]*[│║].*[│║][ \t]*$/;

function parseTableBlock(blockLines) {
  const rows = [];
  let headerEnd = -1;
  for (const raw of blockLines) {
    const v = raw.replace(ANSI_STRIP_RE, "");
    if (TABLE_TOP_RE.test(v) || TABLE_BOT_RE.test(v)) continue;
    if (TABLE_SEP_RE.test(v)) {
      if (headerEnd === -1 && rows.length > 0) headerEnd = rows.length;
      continue;
    }
    if (!TABLE_ROW_RE.test(v)) continue;
    const inner = v.replace(/^[ \t]*[│║]/, "").replace(/[│║][ \t]*$/, "");
    rows.push(inner.split(/[│║]/).map((c) => c.trim()));
  }
  return { rows, headerEnd };
}

function renderTable(parsed) {
  const { rows, headerEnd } = parsed;
  if (!rows.length) return "";
  const hEnd = headerEnd > 0 ? headerEnd : 0;
  const parts = ['<div class="tui-table-wrap"><table class="tui-table">'];
  if (hEnd > 0) {
    parts.push("<thead>");
    for (let r = 0; r < hEnd; r++) {
      parts.push("<tr>");
      for (const c of rows[r]) parts.push("<th>" + escapeHTML(c) + "</th>");
      parts.push("</tr>");
    }
    parts.push("</thead>");
  }
  parts.push("<tbody>");
  for (let r = hEnd; r < rows.length; r++) {
    parts.push("<tr>");
    for (const c of rows[r]) parts.push("<td>" + escapeHTML(c) + "</td>");
    parts.push("</tr>");
  }
  parts.push("</tbody></table></div>");
  return parts.join("");
}

function extractTables(text) {
  const lines = text.split("\n");
  const tables = [];
  const out = [];
  let i = 0;
  while (i < lines.length) {
    const v = lines[i].replace(ANSI_STRIP_RE, "");
    if (TABLE_TOP_RE.test(v)) {
      let j = i + 1;
      while (j < lines.length) {
        const vj = lines[j].replace(ANSI_STRIP_RE, "");
        if (TABLE_BOT_RE.test(vj)) break;
        // A blank line or non-table line before the bottom frame means this
        // wasn't a real table — give up and emit lines verbatim.
        if (!TABLE_ROW_RE.test(vj) && !TABLE_SEP_RE.test(vj)) { j = -1; break; }
        j++;
      }
      if (j > 0 && j < lines.length) {
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
    out.push(lines[i]);
    i++;
  }
  return { text: out.join("\n"), tables };
}

function wrapRuleLines(text) {
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const visible = lines[i].replace(ANSI_STRIP_RE, "");
    const ruleChars = visible.replace(/\s/g, "");
    if (!/^[─-]+$/.test(ruleChars)) continue;
    if (ruleChars.length < 8) continue;
    lines[i] = RULE_OPEN + RULE_CLOSE;
  }
  return lines.join("\n");
}

function markImagePaths(root, cwd) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const nodes = [];
  while (walker.nextNode()) nodes.push(walker.currentNode);

  for (const node of nodes) {
    const text = node.nodeValue;
    IMAGE_PATH_RE.lastIndex = 0;
    let m, last = 0;
    const frag = document.createDocumentFragment();
    while ((m = IMAGE_PATH_RE.exec(text)) !== null) {
      if (!previewableImagePath(m[0])) continue;
      if (m.index > last) frag.appendChild(document.createTextNode(text.slice(last, m.index)));
      const span = document.createElement("span");
      span.className = "image-path";
      span.textContent = m[0];
      span.dataset.path = m[0];
      if (cwd) span.dataset.cwd = cwd;
      span.title = "Command-click to preview image";
      frag.appendChild(span);
      last = IMAGE_PATH_RE.lastIndex;
    }
    if (last === 0) continue;
    if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
    node.parentNode.replaceChild(frag, node);
  }
}

export function setContent(text, opts) {
  const pre = $("content");
  const atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 60;
  const extracted = extractTables(text);
  let html = ansiToHTML(wrapRuleLines(extracted.text))
    .replaceAll(RULE_OPEN, '<span class="tui-rule" role="separator">')
    .replaceAll(RULE_CLOSE, "</span>");
  if (extracted.tables.length) {
    html = html.replace(TABLE_PLACEHOLDER_RE, (_, n) => extracted.tables[+n] || "");
  }
  pre.innerHTML = html;
  markImagePaths(pre, opts && opts.cwd);
  if (atBottom) pre.scrollTop = pre.scrollHeight;
}

// Exported for tests.
export const __test__ = { extractTables, parseTableBlock, renderTable };
