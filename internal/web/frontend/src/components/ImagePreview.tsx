// ImagePreview — the React port of app.js's image-path lightbox
// (ensureImagePreview / previewImagePath). A Cmd+click (desktop) or 550 ms
// long-press (mobile) on an .image-path span opens this overlay; it fetches the
// image from /api/image and shows it centered over a dimmed backdrop.
//
// Parity notes (app.js lines 511–549):
//   - overlay is .image-preview with the HTML `hidden` attribute (CSS:
//     `.image-preview[hidden] { display: none }`); it lives at document.body
//     level — here a portal into document.body preserves that stacking + the
//     verbatim class names CSS depends on.
//   - card holds: a round .image-preview-close button (X svg), an
//     <img alt="preview">, and an .image-preview-path label.
//   - previewImagePath built the src as
//       /api/image?path=<path>[&cwd=<cwd>][&peer=<peer>]   (URLSearchParams)
//     and set the path label text to the raw path. App builds the src via
//     buildImageSrc() below and passes it down, so the wire shape is identical.
//   - close on: backdrop click (e.target === overlay), the close button, and
//     Escape — but ONLY when not hidden (the original guards `!overlay.hidden`).
//   - closing clears <img>.src (the original did img.removeAttribute("src")).
//     Here `src == null` means hidden, so the <img> simply isn't given a src.

import { useEffect } from "react";
import { createPortal } from "react-dom";

// buildImageSrc mirrors app.js previewImagePath's URL construction exactly:
//   const qs = new URLSearchParams({ path });
//   if (cwd) qs.set("cwd", cwd);
//   if (peer) qs.set("peer", peer);
//   "/api/image?" + qs.toString();
export function buildImageSrc(path: string, cwd: string, peer: string): string {
  const qs = new URLSearchParams({ path });
  if (cwd) qs.set("cwd", cwd);
  if (peer) qs.set("peer", peer);
  return "/api/image?" + qs.toString();
}

export function buildImageDownloadHref(path: string, cwd: string, peer: string): string {
  const qs = new URLSearchParams({ path, download: "1" });
  if (cwd) qs.set("cwd", cwd);
  if (peer) qs.set("peer", peer);
  return "/api/image?" + qs.toString();
}

export interface ImagePreviewProps {
  /** The /api/image src to show, or null when the lightbox is closed. */
  src: string | null;
  /** The /api/image download href, or null when the lightbox is closed. */
  downloadHref?: string | null;
  /** Raw image path shown in the .image-preview-path label (parity with app.js). */
  path?: string;
  /** Invoked on backdrop click / close button / Escape — App clears `src`. */
  onClose: () => void;
}

export default function ImagePreview({ src, downloadHref, path, onClose }: ImagePreviewProps) {
  const hidden = src == null;

  // Escape closes only when the overlay is open — the original registered a
  // single document keydown listener guarded by `!overlay.hidden`.
  useEffect(() => {
    if (hidden) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [hidden, onClose]);

  const overlay = (
    <div
      className="image-preview"
      hidden={hidden}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="image-preview-card">
        <button
          className="image-preview-close"
          type="button"
          title="close"
          aria-label="close image preview"
          onClick={onClose}
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.5"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M18 6 6 18" />
            <path d="m6 6 12 12" />
          </svg>
        </button>
        {downloadHref ? (
          <a
            className="image-preview-download"
            href={downloadHref}
            title="download"
            aria-label="download image"
            onClick={(e) => e.stopPropagation()}
          >
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.3"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <path d="M12 3v12" />
              <path d="m7 10 5 5 5-5" />
              <path d="M5 21h14" />
            </svg>
          </a>
        ) : null}
        {/* No src attribute when closed mirrors removeAttribute("src"). */}
        {src == null ? <img alt="preview" /> : <img alt="preview" src={src} />}
        <div className="image-preview-path">{path ?? ""}</div>
      </div>
    </div>
  );

  return createPortal(overlay, document.body);
}
