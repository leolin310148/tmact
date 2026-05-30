import { describe, it, expect } from "vitest";
import {
  render,
  ansiToHTML,
  ansi256,
  wrapRuleLines,
  extractTables,
  extractURLs,
  joinWrappedFrames,
  parseTableBlock,
  renderTable,
  previewableImagePath,
  markImagePaths,
  IMAGE_PATH_RE,
  __test__,
} from "./render";

// Private-use-area placeholder markers, kept byte-identical to terminal.js so
// the tests assert the same internal contract the renderer relies on.
const RULE_OPEN = "", RULE_CLOSE = "";
const TABLE_OPEN = "", TABLE_CLOSE = "";
const URL_OPEN = "", URL_SEP = "", URL_CLOSE = "";

const ESC = "\x1b";

// ---------------------------------------------------------------------------
// ANSI SGR → HTML  (spec §6 items 19-20)
// ---------------------------------------------------------------------------
describe("ansiToHTML", () => {
  it("escapes &<> BEFORE styling, no markup injection from pane text", () => {
    expect(ansiToHTML("a & b < c > d")).toBe("a &amp; b &lt; c &gt; d");
  });

  it("plain text with no SGR is passed through unstyled", () => {
    expect(ansiToHTML("hello world")).toBe("hello world");
  });

  it("16-colour foreground uses the Tango palette", () => {
    // 31 = red foreground → ANSI16[1] = #cc0000
    expect(ansiToHTML(ESC + "[31mred" + ESC + "[0m")).toBe(
      '<span style="color:#cc0000">red</span>',
    );
  });

  it("bright foreground (90-97) maps via offset -82", () => {
    // 92 → ANSI16[10] = #8ae234
    expect(ansiToHTML(ESC + "[92mx")).toBe('<span style="color:#8ae234">x</span>');
  });

  it("16-colour background (40-47) and bright background (100-107)", () => {
    // 41 → ANSI16[1] background
    expect(ansiToHTML(ESC + "[41mx")).toBe('<span style="background:#cc0000">x</span>');
    // 102 → ANSI16[10] background
    expect(ansiToHTML(ESC + "[102mx")).toBe('<span style="background:#8ae234">x</span>');
  });

  it("256-colour foreground (38;5;n)", () => {
    // 38;5;9 → ANSI16[9] = #ef2929
    expect(ansiToHTML(ESC + "[38;5;9mx")).toBe('<span style="color:#ef2929">x</span>');
    // 38;5;250 → grayscale ramp
    expect(ansiToHTML(ESC + "[38;5;250mx")).toBe(
      '<span style="color:' + ansi256(250) + '">x</span>',
    );
  });

  it("RGB colour (38;2;r;g;b) clamps each channel 0..255", () => {
    expect(ansiToHTML(ESC + "[38;2;10;20;300mx")).toBe(
      '<span style="color:rgb(10,20,255)">x</span>',
    );
  });

  it("malformed truecolor subcodes are dropped without crashing", () => {
    // 38 with no following 5/2 → no colour applied
    expect(ansiToHTML(ESC + "[38mx")).toBe("x");
  });

  it("bold → font-weight:700, dim → opacity:.6, italic, underline", () => {
    expect(ansiToHTML(ESC + "[1mx")).toBe('<span style="font-weight:700">x</span>');
    expect(ansiToHTML(ESC + "[2mx")).toBe('<span style="opacity:.6">x</span>');
    expect(ansiToHTML(ESC + "[3mx")).toBe('<span style="font-style:italic">x</span>');
    expect(ansiToHTML(ESC + "[4mx")).toBe('<span style="text-decoration:underline">x</span>');
  });

  it("reverse swaps fg/bg, defaulting to var(--bg)/var(--fg)", () => {
    expect(ansiToHTML(ESC + "[7mx")).toBe(
      '<span style="color:var(--bg);background:var(--fg)">x</span>',
    );
    // explicit fg+bg then reverse swaps them
    expect(ansiToHTML(ESC + "[31;42;7mx")).toBe(
      '<span style="color:#4e9a06;background:#cc0000">x</span>',
    );
  });

  it("reset (0 / empty params) clears all state", () => {
    expect(ansiToHTML(ESC + "[31ma" + ESC + "[0mb")).toBe(
      '<span style="color:#cc0000">a</span>b',
    );
    // empty params == [0]
    expect(ansiToHTML(ESC + "[31ma" + ESC + "[mb")).toBe(
      '<span style="color:#cc0000">a</span>b',
    );
  });

  it("individual unset codes (22/23/24/27/39/49)", () => {
    expect(ansiToHTML(ESC + "[1;22mx")).toBe("x"); // bold then unbold
    expect(ansiToHTML(ESC + "[31;39mx")).toBe("x"); // fg then default fg
    expect(ansiToHTML(ESC + "[41;49mx")).toBe("x"); // bg then default bg
  });

  it("non-SGR CSI/OSC escapes are dropped, surrounding text preserved", () => {
    // \x1b[2J (clear screen) is a non-SGR CSI → dropped
    expect(ansiToHTML("a" + ESC + "[2Jb")).toBe("ab");
    // OSC title set → dropped
    expect(ansiToHTML("a" + ESC + "]0;title\x07b")).toBe("ab");
  });

  it("trailing ANSI state is re-emitted onto the tail text", () => {
    // colour set mid-string, no reset → tail keeps the style
    expect(ansiToHTML("plain" + ESC + "[31mred")).toBe(
      'plain<span style="color:#cc0000">red</span>',
    );
  });
});

describe("ansi256", () => {
  it("0..15 map to the base palette", () => {
    expect(ansi256(0)).toBe("#000000");
    expect(ansi256(1)).toBe("#cc0000");
    expect(ansi256(15)).toBe("#eeeeec");
  });
  it("16..231 are the 6x6x6 cube", () => {
    expect(ansi256(16)).toBe("#000000");
    expect(ansi256(21)).toBe("#0000ff");
    expect(ansi256(231)).toBe("#ffffff");
  });
  it("232..255 are the grayscale ramp", () => {
    expect(ansi256(232)).toBe("#080808");
    expect(ansi256(255)).toBe("#eeeeee");
  });
});

// ---------------------------------------------------------------------------
// Horizontal rules  (spec §6 item 23)
// ---------------------------------------------------------------------------
describe("wrapRuleLines", () => {
  it("replaces a run of ≥8 box-drawing dashes with the rule placeholder", () => {
    const line = "─".repeat(10);
    expect(wrapRuleLines(line)).toBe(RULE_OPEN + RULE_CLOSE);
  });

  it("replaces a run of ≥8 ASCII hyphens with the rule placeholder", () => {
    expect(wrapRuleLines("--------")).toBe(RULE_OPEN + RULE_CLOSE);
  });

  it("leaves runs shorter than 8 chars untouched", () => {
    expect(wrapRuleLines("-------")).toBe("-------");
    expect(wrapRuleLines("─".repeat(7))).toBe("─".repeat(7));
  });

  it("ignores whitespace when counting rule chars", () => {
    // 8 dashes with interspersed spaces still counts as a rule
    expect(wrapRuleLines("---- ----")).toBe(RULE_OPEN + RULE_CLOSE);
  });

  it("does not match lines with non-rule characters", () => {
    expect(wrapRuleLines("--------x")).toBe("--------x");
  });

  it("strips ANSI before measuring the rule", () => {
    const line = ESC + "[31m" + "─".repeat(10) + ESC + "[0m";
    expect(wrapRuleLines(line)).toBe(RULE_OPEN + RULE_CLOSE);
  });

  it("render() turns the rule placeholder into a tui-rule span", () => {
    const out = render("─".repeat(10));
    expect(out).toBe('<span class="tui-rule" role="separator"></span>');
  });
});

// ---------------------------------------------------------------------------
// URL extraction  (spec §6 item 21)
// ---------------------------------------------------------------------------
describe("extractURLs", () => {
  it("extracts a plain URL and replaces it with a placeholder", () => {
    const { text, urls } = extractURLs("see https://example.com/x now");
    expect(urls).toEqual(["https://example.com/x"]);
    expect(text).toBe("see " + URL_OPEN + "0" + URL_SEP + "https://example.com/x" + URL_CLOSE + " now");
  });

  it("strips trailing punctuation from href but keeps it visible", () => {
    const { text, urls } = extractURLs("(see https://example.com/x).");
    expect(urls).toEqual(["https://example.com/x"]);
    // trailing ")." is emitted AFTER the placeholder close
    expect(text).toBe("(see " + URL_OPEN + "0" + URL_SEP + "https://example.com/x" + URL_CLOSE + ").");
  });

  it("joins a soft-wrapped URL (indent continuation) into the href", () => {
    // The continuation line must start (after the indent) with a URL-path lead
    // char from [/?&=#+%~@:.-] for the soft-wrap branch to fire — tmux wraps a
    // long github link right at a path boundary, so the next visible char is /.
    const raw = "https://github.com/owner/repo/blob/main\n    /path/to/file.go";
    const { text, urls } = extractURLs(raw);
    expect(urls).toEqual(["https://github.com/owner/repo/blob/main/path/to/file.go"]);
    // the visible body keeps the newline + indent
    expect(text).toContain(URL_OPEN + "0" + URL_SEP);
    expect(text).toContain("https://github.com/owner/repo/blob/main\n    /path/to/file.go");
  });

  it("strips mid-URL ANSI from the href and re-emits it after the placeholder", () => {
    const raw = "https://github.com/x" + ESC + "[0m" + "/y";
    const { text, urls } = extractURLs(raw);
    expect(urls).toEqual(["https://github.com/x/y"]);
    // trailing ANSI re-appended after URL_CLOSE
    expect(text).toBe(URL_OPEN + "0" + URL_SEP + "https://github.com/x/y" + URL_CLOSE + ESC + "[0m");
  });

  it("does not glue CJK / prose onto the URL", () => {
    const { urls } = extractURLs("https://example.com/path 中文說明");
    expect(urls).toEqual(["https://example.com/path"]);
  });

  it("render() wraps the URL placeholder in a tui-link anchor with target/rel", () => {
    const out = render("https://example.com/x");
    expect(out).toBe(
      '<a href="https://example.com/x" target="_blank" rel="noopener noreferrer" class="tui-link">https://example.com/x</a>',
    );
  });

  it("render() escapes the href attribute", () => {
    const out = render("https://example.com/a&b");
    expect(out).toContain('href="https://example.com/a&amp;b"');
  });
});

// ---------------------------------------------------------------------------
// Box-art table extraction  (spec §6 item 22)
// ---------------------------------------------------------------------------
describe("joinWrappedFrames", () => {
  it("merges a wrap-continuation frame line back onto the preceding frame", () => {
    const lines = [
      "┌───┐",          // ┌───┐ (start)
      "───┐",                // ───┐ (continuation of the top)
    ];
    const out = joinWrappedFrames(lines);
    expect(out).toHaveLength(1);
    expect(out[0]).toBe("┌───┐───┐");
  });

  it("does not merge a non-continuation line", () => {
    const lines = ["┌──┐", "regular text"];
    expect(joinWrappedFrames(lines)).toEqual(lines);
  });
});

describe("parseTableBlock / renderTable / extractTables", () => {
  const top = "┌───┬───┐";    // ┌───┬───┐
  const sep = "├───┼───┤";    // ├───┼───┤
  const bot = "└───┴───┘";    // └───┴───┘
  const row = (a: string, b: string): string => "│ " + a + " │ " + b + " │"; // │ a │ b │

  it("column count = (#┬)+1", () => {
    const parsed = parseTableBlock([top, row("A", "B"), bot]);
    expect(parsed.rows).toEqual([["A", "B"]]);
  });

  it("├─┤ separator marks the header end (thead emitted)", () => {
    const parsed = parseTableBlock([top, row("H1", "H2"), sep, row("a", "b"), bot]);
    expect(parsed.headerEnd).toBe(1);
    const html = renderTable(parsed);
    expect(html).toContain("<thead><tr><th>H1</th><th>H2</th></tr></thead>");
    expect(html).toContain("<tbody><tr><td>a</td><td>b</td></tr></tbody>");
    expect(html).toContain('<div class="tui-table-wrap"><table class="tui-table">');
  });

  it("no separator → no thead (all rows in tbody)", () => {
    const parsed = parseTableBlock([top, row("a", "b"), row("c", "d"), bot]);
    expect(parsed.headerEnd).toBe(-1);
    const html = renderTable(parsed);
    expect(html).not.toContain("<thead>");
    expect(html).toContain("<tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody>");
  });

  it("cell-count mismatch invalidates the WHOLE block (raw fallback)", () => {
    // a row with 3 cells under a 2-column table
    const bad = "│ a │ b │ c │";
    const parsed = parseTableBlock([top, bad, bot]);
    expect(parsed.rows).toEqual([]);
    expect(parsed.headerEnd).toBe(-1);
  });

  it("renderTable escapes cell contents", () => {
    const parsed = parseTableBlock([top, row("<x>", "a&b"), bot]);
    const html = renderTable(parsed);
    expect(html).toContain("<td>&lt;x&gt;</td>");
    expect(html).toContain("<td>a&amp;b</td>");
  });

  it("extractTables replaces a valid table with a TABLE placeholder", () => {
    const text = ["before", top, row("a", "b"), bot, "after"].join("\n");
    const { text: outText, tables } = extractTables(text);
    expect(tables).toHaveLength(1);
    expect(outText).toBe(["before", TABLE_OPEN + "0" + TABLE_CLOSE, "after"].join("\n"));
  });

  it("extractTables: doubled top frame discards the outer, still extracts the inner", () => {
    const text = [top, top, row("a", "b"), bot].join("\n");
    const { tables } = extractTables(text);
    // Parity with terminal.js: scanning from the first top hits another top
    // before a bottom → that (outer) frame bails as broken nesting (j = -1),
    // but the scan then restarts at the second top, where [top, row, bot] is a
    // valid single table. So exactly one table is extracted.
    expect(tables).toHaveLength(1);
  });

  it("extractTables falls back to raw on cell-count mismatch", () => {
    const bad = "│ a │ b │ c │";
    const text = [top, bad, bot].join("\n");
    const { text: outText, tables } = extractTables(text);
    expect(tables).toHaveLength(0);
    expect(outText).toBe(text);
  });

  it("renderTable on an empty parse returns empty string", () => {
    expect(renderTable({ rows: [], headerEnd: -1 })).toBe("");
  });

  it("render() splices the real <table> back in", () => {
    const text = [top, row("a", "b"), bot].join("\n");
    const out = render(text);
    expect(out).toContain('<table class="tui-table">');
    expect(out).toContain("<td>a</td>");
    expect(out).not.toContain(TABLE_OPEN);
  });
});

// ---------------------------------------------------------------------------
// Image paths  (spec §6 item 24)
// ---------------------------------------------------------------------------
describe("previewableImagePath", () => {
  it("rejects ~/ prefixed paths (not previewable)", () => {
    expect(previewableImagePath("~/pic.png")).toBe(false);
  });
  it("accepts absolute and relative paths", () => {
    expect(previewableImagePath("/abs/pic.png")).toBe(true);
    expect(previewableImagePath("./rel/pic.png")).toBe(true);
    expect(previewableImagePath("pic.png")).toBe(true);
  });
  it("accepts file:// scheme but rejects other schemes", () => {
    expect(previewableImagePath("file:///abs/pic.png")).toBe(true);
    expect(previewableImagePath("https://example.com/pic.png")).toBe(false);
  });
});

describe("IMAGE_PATH_RE", () => {
  it("matches common image extensions case-insensitively", () => {
    for (const ext of ["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "PNG", "JPG"]) {
      IMAGE_PATH_RE.lastIndex = 0;
      expect(IMAGE_PATH_RE.test("/x/y.foo." + ext)).toBe(true);
    }
  });
  it("does not match non-image extensions", () => {
    IMAGE_PATH_RE.lastIndex = 0;
    expect(IMAGE_PATH_RE.test("/x/y.txt")).toBe(false);
  });
});

describe("markImagePaths", () => {
  it("wraps a previewable image path in an image-path span with data attrs", () => {
    const root = document.createElement("pre");
    root.textContent = "open /home/u/pic.png please";
    markImagePaths(root, "/home/u", "z13");
    const span = root.querySelector("span.image-path");
    expect(span).not.toBeNull();
    expect(span?.textContent).toBe("/home/u/pic.png");
    expect((span as HTMLElement).dataset.path).toBe("/home/u/pic.png");
    expect((span as HTMLElement).dataset.cwd).toBe("/home/u");
    expect((span as HTMLElement).dataset.peer).toBe("z13");
    expect((span as HTMLElement).title).toBe("Command-click to preview image");
    // surrounding text preserved
    expect(root.textContent).toBe("open /home/u/pic.png please");
  });

  it("omits data-cwd / data-peer when not provided", () => {
    const root = document.createElement("pre");
    root.textContent = "/abs/pic.png";
    markImagePaths(root);
    const span = root.querySelector("span.image-path") as HTMLElement;
    expect(span.dataset.cwd).toBeUndefined();
    expect(span.dataset.peer).toBeUndefined();
  });

  it("does NOT wrap a ~/ path (not previewable)", () => {
    const root = document.createElement("pre");
    root.textContent = "~/pic.png";
    markImagePaths(root, "/home/u");
    expect(root.querySelector("span.image-path")).toBeNull();
    expect(root.textContent).toBe("~/pic.png");
  });

  it("leaves text nodes without an image path untouched", () => {
    const root = document.createElement("pre");
    root.textContent = "no images here";
    markImagePaths(root);
    expect(root.childNodes).toHaveLength(1);
    expect(root.childNodes[0]?.nodeType).toBe(Node.TEXT_NODE);
  });
});

// ---------------------------------------------------------------------------
// Placeholder replacement ordering & integration  (spec §6 items 25-26)
// ---------------------------------------------------------------------------
describe("render integration", () => {
  it("handles rules + tables + URLs together (private-use markers survive escaping)", () => {
    const top = "┌───┐";  // ┌───┐ (1 column)
    const bot = "└───┘";  // └───┘
    const row = "│ hi │";                 // │ hi │
    const text = [
      "https://example.com/a",
      "─".repeat(10),
      top,
      row,
      bot,
    ].join("\n");
    const out = render(text);
    expect(out).toContain('class="tui-link"');
    expect(out).toContain('class="tui-rule"');
    expect(out).toContain('class="tui-table"');
    // no leftover PUA markers in the final HTML
    for (const marker of [RULE_OPEN, RULE_CLOSE, TABLE_OPEN, TABLE_CLOSE, URL_OPEN, URL_SEP, URL_CLOSE]) {
      expect(out).not.toContain(marker);
    }
  });

  it("escapes html in non-URL/non-table text", () => {
    expect(render("a < b & c > d")).toBe("a &lt; b &amp; c &gt; d");
  });

  // Regression: tmux capture-pane pads a prompt-idle pane with trailing empty
  // rows. Rendering them verbatim made #content overflow and the stick-to-bottom
  // auto-scroll parked on the blank tail, hiding the real output. render() now
  // strips trailing blank ROWS so short output fits the pane (no scroll).
  it("strips trailing blank lines (tmux pane padding)", () => {
    expect(render("prompt\n❯\n\n\n\n\n")).toBe("prompt\n❯");
  });

  it("strips trailing rows that are whitespace-only too", () => {
    expect(render("a\nb\n   \n\t\n  ")).toBe("a\nb");
  });

  it("keeps the last non-blank line's own trailing spaces, and leading/interior blanks", () => {
    // leading blank kept, interior blank kept, "b   " keeps its trailing spaces,
    // only the trailing empty rows after it are removed.
    expect(render("\na\n\nb   \n\n")).toBe("\na\n\nb   ");
  });

  it("strips trailing blank rows even when they carry only ANSI escapes (capture-pane -e re-asserts SGR)", () => {
    // tmux `-e` re-asserts SGR state on visually-empty cells, so a padding row
    // can arrive as e.g. "\x1b[49m   " instead of pure spaces. These must still
    // be trimmed, or the scroll bug resurfaces for colored output. A plain
    // [ \t]-only check would miss them.
    const E = "\x1b";
    expect(render(`prompt\n❯\n${E}[49m   \n${E}[49m   `)).toBe("prompt\n❯");
  });

  it("strips trailing CRLF blank rows (the \\r? branch of the trim)", () => {
    // Defensive: the real tmux→server→client path is LF-only, but the trim
    // tolerates CRLF so a refactor that drops \\r? would be caught here.
    expect(render("a\r\nb\r\n\r\n\r\n")).toBe("a\r\nb");
  });

  it("collapses an all-blank pane to empty, but keeps a lone whitespace line with no trailing newline", () => {
    expect(render("\n\n\n")).toBe("");
    // A blank-row run is matched per "\n…": a first whitespace-only row with no
    // preceding newline has nothing to anchor on, so it survives verbatim.
    expect(render("   \n  \n")).toBe("   ");
    expect(render("   ")).toBe("   ");
  });

  it("trims tmux padding after a table/rule/URL without dropping the block", () => {
    // The trim runs BEFORE extractTables/wrapRuleLines/extractURLs, so a
    // structured block that ends the output (then padded with blank rows) must
    // survive intact — the common "finished command output" case.
    const tableOut = render(["┌───┐", "│ hi │", "└───┘", "", "", ""].join("\n"));
    expect(tableOut).toContain('class="tui-table"');
    expect(tableOut).toContain("<td>hi</td>");
    expect(render("─".repeat(10) + "\n\n")).toBe(
      '<span class="tui-rule" role="separator"></span>',
    );
    expect(render("https://example.com/x\n\n\n")).toContain('class="tui-link"');
  });
});

// ---------------------------------------------------------------------------
// __test__ surface parity (mirrors terminal.js's exported helper bundle)
// ---------------------------------------------------------------------------
describe("__test__ export bundle", () => {
  it("exposes the same helper names as terminal.js", () => {
    expect(Object.keys(__test__).sort()).toEqual(
      ["extractTables", "extractURLs", "joinWrappedFrames", "parseTableBlock", "renderTable"].sort(),
    );
  });
  it("the bundled functions are the same references as the named exports", () => {
    expect(__test__.extractTables).toBe(extractTables);
    expect(__test__.parseTableBlock).toBe(parseTableBlock);
    expect(__test__.renderTable).toBe(renderTable);
    expect(__test__.joinWrappedFrames).toBe(joinWrappedFrames);
    expect(__test__.extractURLs).toBe(extractURLs);
  });
});
