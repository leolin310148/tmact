export const $ = (id) => document.getElementById(id);

export const isMobile = () => window.matchMedia("(max-width: 760px)").matches;

export function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  for (const k in (attrs || {})) {
    if (k === "class") el.className = attrs[k];
    else if (k === "text") el.textContent = attrs[k];
    else el.setAttribute(k, attrs[k]);
  }
  for (const kid of kids) if (kid) el.appendChild(kid);
  return el;
}

export function escapeHTML(s) {
  return s.replace(/[&<>]/g, (c) => (c === "&" ? "&amp;" : c === "<" ? "&lt;" : "&gt;"));
}

export function clamp(n, min, max) {
  if (max < min) return min;
  return Math.max(min, Math.min(max, n));
}
