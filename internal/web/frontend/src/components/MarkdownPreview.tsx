import MarkdownIt from "markdown-it";
import type Token from "markdown-it/lib/token.mjs";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { escapeAttribute, escapeHTML } from "../lib/dom";
import { getCachedMermaidSVG, renderCachedMermaidBlock, renderMermaidDiagrams } from "../lib/mermaid";
import { buildImageSrc } from "./ImagePreview";

export interface MarkdownPreviewTarget {
  path: string;
  cwd: string;
  peer: string;
}

interface MarkdownPreviewResponse {
  content: string;
  path: string;
  baseDir: string;
  filename: string;
}

export function buildMarkdownSrc(path: string, cwd: string, peer: string): string {
  const qs = new URLSearchParams({ path });
  if (cwd) qs.set("cwd", cwd);
  if (peer) qs.set("peer", peer);
  return "/api/markdown?" + qs.toString();
}

export function buildMarkdownDownloadHref(path: string, cwd: string, peer: string): string {
  const qs = new URLSearchParams({ path });
  if (cwd) qs.set("cwd", cwd);
  if (peer) qs.set("peer", peer);
  return "/api/file?" + qs.toString();
}

const URL_SCHEME_RE = /^[A-Za-z][A-Za-z0-9+.-]*:\/\//;
const IMAGE_EXT_RE = /\.(?:png|jpe?g|gif|webp|bmp|svg)$/i;
const MERMAID_LANG_RE = /^mermaid(?:\s|$)/i;

function isSafeLinkHref(href: string): boolean {
  return href.startsWith("#") || /^https?:\/\//i.test(href);
}

function removeAttr(token: Token, name: string): void {
  const idx = token.attrIndex(name);
  if (idx >= 0) token.attrs?.splice(idx, 1);
}

function isPreviewableMarkdownImage(src: string): boolean {
  if (!src || src.startsWith("~/") || /[\r\n\x00]/.test(src)) return false;
  const scheme = URL_SCHEME_RE.exec(src);
  if (scheme && scheme[0].toLowerCase() !== "file://") return false;
  return IMAGE_EXT_RE.test(src);
}

function imageAlt(tokens: Token[], idx: number): string {
  return tokens[idx]?.content ?? "";
}

export function renderMarkdownPreview(content: string, baseDir: string, peer: string): string {
  const md = new MarkdownIt({
    html: false,
    linkify: false,
    typographer: false,
  });

  const defaultFence = md.renderer.rules.fence;
  md.renderer.rules.fence = (tokens, idx, options, env, self) => {
    const token = tokens[idx];
    const info = token?.info.trim() ?? "";
    if (token && MERMAID_LANG_RE.test(info)) {
      const cachedSVG = getCachedMermaidSVG(token.content);
      if (cachedSVG) {
        return renderCachedMermaidBlock(token.content, cachedSVG);
      }
      return `<div class="markdown-preview-mermaid" data-mermaid-state="pending" data-mermaid-source="${escapeAttribute(token.content)}"><div class="markdown-preview-mermaid-loading">Rendering diagram...</div></div>`;
    }
    return defaultFence ? defaultFence(tokens, idx, options, env, self) : self.renderToken(tokens, idx, options);
  };

  const defaultLinkOpen = md.renderer.rules.link_open;
  md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
    const token = tokens[idx];
    const href = token?.attrGet("href") ?? "";
    if (token && !isSafeLinkHref(href)) {
      removeAttr(token, "href");
      token.attrJoin("class", "markdown-preview-disabled-link");
    }
    return defaultLinkOpen ? defaultLinkOpen(tokens, idx, options, env, self) : self.renderToken(tokens, idx, options);
  };

  md.renderer.rules.image = (tokens, idx) => {
    const src = tokens[idx]?.attrGet("src") ?? "";
    const alt = imageAlt(tokens, idx);
    if (!isPreviewableMarkdownImage(src)) return escapeHTML(alt);
    const safeSrc = buildImageSrc(src, baseDir, peer);
    const title = tokens[idx]?.attrGet("title");
    return `<img src="${escapeHTML(safeSrc)}" alt="${escapeHTML(alt)}"${
      title ? ` title="${escapeHTML(title)}"` : ""
    }>`;
  };

  return md.render(content);
}

export interface MarkdownPreviewProps {
  target: MarkdownPreviewTarget | null;
  onClose: () => void;
}

export default function MarkdownPreview({ target, onClose }: MarkdownPreviewProps) {
  const [data, setData] = useState<MarkdownPreviewResponse | null>(null);
  const [error, setError] = useState<string>("");
  const contentRef = useRef<HTMLDivElement | null>(null);
  const hidden = target == null;

  useEffect(() => {
    if (!target) {
      setData(null);
      setError("");
      return;
    }
    const ac = new AbortController();
    setData(null);
    setError("");
    void fetch(buildMarkdownSrc(target.path, target.cwd, target.peer), { signal: ac.signal })
      .then(async (resp) => {
        if (!resp.ok) throw new Error((await resp.text()) || `HTTP ${resp.status}`);
        return (await resp.json()) as MarkdownPreviewResponse;
      })
      .then(setData)
      .catch((err: unknown) => {
        if (ac.signal.aborted) return;
        setError(err instanceof Error ? err.message : String(err));
      });
    return () => ac.abort();
  }, [target]);

  useEffect(() => {
    if (hidden) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [hidden, onClose]);

  const body = useMemo(() => {
    if (!data) return "";
    return renderMarkdownPreview(data.content, data.baseDir, target?.peer ?? "");
  }, [data, target?.peer]);

  useLayoutEffect(() => {
    if (!data || error || !contentRef.current) return;
    contentRef.current.innerHTML = body;
    void renderMermaidDiagrams(contentRef.current);
  }, [body, data, error]);

  const downloadHref = data
    ? buildMarkdownDownloadHref(data.path, "", target?.peer ?? "")
    : target
      ? buildMarkdownDownloadHref(target.path, target.cwd, target.peer)
      : null;
  const label = data?.path ?? target?.path ?? "";

  const overlay = (
    <div
      className="markdown-preview"
      hidden={hidden}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="markdown-preview-card">
        <button
          className="image-preview-close"
          type="button"
          title="close"
          aria-label="close markdown preview"
          onClick={onClose}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" aria-hidden="true">
            <path d="M18 6 6 18" />
            <path d="m6 6 12 12" />
          </svg>
        </button>
        {downloadHref ? (
          <a className="image-preview-download" href={downloadHref} title="download" aria-label="download markdown" onClick={(e) => e.stopPropagation()}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.3" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M12 3v12" />
              <path d="m7 10 5 5 5-5" />
              <path d="M5 21h14" />
            </svg>
          </a>
        ) : null}
        <div className="markdown-preview-body">
          {error ? <div className="markdown-preview-error">{error}</div> : null}
          {!error && !data ? <div className="markdown-preview-loading">Loading...</div> : null}
          {!error && data ? <div ref={contentRef} className="markdown-preview-content" /> : null}
        </div>
        <div className="image-preview-path">{label}</div>
      </div>
    </div>
  );

  return createPortal(overlay, document.body);
}
