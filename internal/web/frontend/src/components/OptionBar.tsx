// OptionBar — the quick-answer bar (#option-bar) for a detected menu prompt,
// ported 1:1 from app.js renderOptions (MIGRATION_SPEC §6 item 56).
//
// app.js:
//   function renderOptions(q) {
//     const bar = $("option-bar");
//     bar.textContent = "";
//     if (!q || !Array.isArray(q.choices)) return;
//     for (const c of q.choices) {
//       if (typeof c.number !== "number") continue;
//       const btn = h("button", { type: "button",
//                                 title: c.label || ("Option " + c.number) },
//         h("span", { class: "opt-num", text: String(c.number) }),
//         c.label ? h("span", { class: "opt-label", text: c.label }) : null);
//       btn.addEventListener("pointerdown", (e) => e.preventDefault());
//       btn.addEventListener("click", () => {
//         if (!wsSend({ t: "text", s: String(c.number) })) {
//           showInputError("not connected — try again");
//         }
//       });
//       bar.appendChild(btn);
//     }
//   }
//
// Behavior preserved:
//   - Falsy question or non-array `choices` → empty bar (no buttons).
//   - One button per choice whose `number` is a number; `[number] label`.
//   - title = label || ("Option " + number); .opt-label span only when labeled.
//   - pointerdown preventDefault keeps the soft keyboard up and focus in place
//     (onPointerDownNoBlur, ARCHITECTURE §6).
//   - Click relays the digit: App's onChoose does
//     `wsSend({ t: "text", s: String(number) })` and surfaces
//     "not connected — try again" on failure.
//
// The bar is CSS-hidden on desktop (`.option-bar { display:none }`) and revealed
// only on phones when non-empty (`@media … .option-bar:not(:empty){display:flex}`),
// so this renders unconditionally and the media query gates visibility — exactly
// like app.js, which ran renderOptions regardless of viewport.

import { onPointerDownNoBlur } from "../lib/dom";
import type { Question } from "../types/server";

interface OptionBarProps {
  /** The detected menu prompt, or null/undefined when none. */
  question: Question | null;
  /** Relays a chosen option number (App wires this to wsSend + error). */
  onChoose: (n: number) => void;
}

export function OptionBar({ question, onChoose }: OptionBarProps) {
  const choices =
    question && Array.isArray(question.choices) ? question.choices : [];

  return (
    <div className="option-bar" id="option-bar">
      {choices.map((c, i) => {
        if (typeof c.number !== "number") return null;
        const num = c.number;
        return (
          <button
            // number is the natural identity; fall back to index for the rare
            // duplicate-number case so React keys stay unique.
            key={"opt-" + num + "-" + i}
            type="button"
            title={c.label || "Option " + num}
            onPointerDown={onPointerDownNoBlur}
            onClick={() => onChoose(num)}
          >
            <span className="opt-num">{String(num)}</span>
            {c.label ? <span className="opt-label">{c.label}</span> : null}
          </button>
        );
      })}
    </div>
  );
}
