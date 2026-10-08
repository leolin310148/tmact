// Pane font picker faces (Settings → Panel font). @font-face is lazy: a woff2
// is only fetched once text actually renders in that family, so the face costs
// nothing until selected. Maple Mono CN is a local Big5 + GB2312 subset split
// into unicode-range chunks (regenerate with scripts/subset-maple-cn.py); the
// chunks are content-hashed under /assets/, served immutable, and kept in the
// service worker's font cache, so each client downloads each chunk once.
import "./maple-cn/maple-cn.css";
import "./fonts.css";
