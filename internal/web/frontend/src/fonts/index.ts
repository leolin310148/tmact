// Pane font picker faces (Settings → Panel font). @font-face is lazy: a woff2
// is only fetched once text actually renders in that family, so the face costs
// nothing until selected. 400 + 700 because pane output renders ANSI bold.
import "@fontsource/maple-mono/400.css";
import "@fontsource/maple-mono/700.css";
import "./fonts.css";
