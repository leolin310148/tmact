import { cleanup, render, screen, waitFor } from "@testing-library/react";
import mermaid from "mermaid";
import { afterEach, describe, expect, it, vi } from "vitest";
import MarkdownPreview, { renderMarkdownPreview } from "./MarkdownPreview";

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async (id: string, source: string) => ({
      svg: `<svg data-testid="mermaid-svg" data-id="${id}"><text>${source}</text></svg>`,
      bindFunctions: vi.fn(),
    })),
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("renderMarkdownPreview", () => {
  it("renders heading, list, code, and table markdown", () => {
    const html = renderMarkdownPreview(
      ["# Title", "", "- item", "", "`code`", "", "| a | b |", "| - | - |", "| c | d |"].join("\n"),
      "/docs",
      "",
    );

    expect(html).toContain("<h1>Title</h1>");
    expect(html).toContain("<li>item</li>");
    expect(html).toContain("<code>code</code>");
    expect(html).toContain("<table>");
    expect(html).toContain("<td>c</td>");
  });

  it("escapes raw HTML instead of creating DOM elements", () => {
    const root = document.createElement("div");
    root.innerHTML = renderMarkdownPreview("<script>alert(1)</script>\n<div>raw</div>", "/docs", "");

    expect(root.querySelector("script")).toBeNull();
    expect(root.querySelector("div")).toBeNull();
    expect(root.textContent).toContain("<script>alert(1)</script>");
    expect(root.textContent).toContain("<div>raw</div>");
  });

  it("rewrites relative markdown images through the image endpoint", () => {
    const root = document.createElement("div");
    root.innerHTML = renderMarkdownPreview("![x](img/a.png)", "/docs", "peer-a");
    const img = root.querySelector("img") as HTMLImageElement | null;

    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe("/api/image?path=img%2Fa.png&cwd=%2Fdocs&peer=peer-a");
  });

  it("does not load unsafe or unsupported image URLs", () => {
    const root = document.createElement("div");
    root.innerHTML = renderMarkdownPreview("![remote](https://example.test/a.png)\n![txt](note.txt)", "/docs", "");

    expect(root.querySelector("img")).toBeNull();
    expect(root.textContent).toContain("remote");
    expect(root.textContent).toContain("txt");
  });

  it("removes unsafe link hrefs", () => {
    const root = document.createElement("div");
    root.innerHTML = renderMarkdownPreview(
      "[relative](guide.md) [file](file:///tmp/a.md) [bad](javascript:alert(1)) [ok](https://example.test)",
      "/docs",
      "",
    );
    const links = root.querySelectorAll("a");

    expect(root.querySelector('a[href^="javascript:"]')).toBeNull();
    expect(root.querySelector('a[href^="file:"]')).toBeNull();
    expect(links[0]?.getAttribute("href")).toBeNull();
    expect(links[links.length - 1]?.getAttribute("href")).toBe("https://example.test");
  });

  it("renders mermaid fenced code as a renderable placeholder", () => {
    const root = document.createElement("div");
    root.innerHTML = renderMarkdownPreview("```mermaid\nflowchart LR\n  A --> B\n```", "/docs", "");
    const block = root.querySelector(".markdown-preview-mermaid") as HTMLElement | null;

    expect(block).not.toBeNull();
    expect(block?.dataset.mermaidState).toBe("pending");
    expect(block?.dataset.mermaidSource).toContain("flowchart LR");
    expect(block?.textContent).toContain("Rendering diagram");
    expect(root.querySelector("pre")).toBeNull();
  });
});

describe("MarkdownPreview", () => {
  it("fetches markdown and renders it in the overlay", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        json: async () => ({
          content: "# Loaded",
          path: "/docs/README.md",
          baseDir: "/docs",
          filename: "README.md",
        }),
      })),
    );

    render(<MarkdownPreview target={{ path: "/docs/README.md", cwd: "", peer: "" }} onClose={vi.fn()} />);

    await waitFor(() => expect(screen.getByRole("heading", { name: "Loaded" })).toBeInTheDocument());
  });

  it("renders mermaid diagrams after markdown is mounted", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        json: async () => ({
          content: "```mermaid\nflowchart LR\n  A --> B\n```",
          path: "/docs/diagram.md",
          baseDir: "/docs",
          filename: "diagram.md",
        }),
      })),
    );

    render(<MarkdownPreview target={{ path: "/docs/diagram.md", cwd: "", peer: "" }} onClose={vi.fn()} />);

    await waitFor(() => expect(screen.getByTestId("mermaid-svg")).toBeInTheDocument());
    expect(mermaid.initialize).toHaveBeenCalledWith(expect.objectContaining({ startOnLoad: false, securityLevel: "strict" }));
    expect(mermaid.render).toHaveBeenCalledWith(expect.stringMatching(/^tmact-mermaid-/), expect.stringContaining("flowchart LR"));
  });
});
