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
// The bar remains empty when no valid choices exist, so CSS can hide it without
// leaving a blank strip. When choices exist it is available at every viewport
// width and preserves the mobile no-blur behavior described above.

import { useId, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { onPointerDownNoBlur } from "../lib/dom";
import type { Question } from "../types/server";

interface OptionBarProps {
  /** The detected menu prompt, or null/undefined when none. */
  question: Question | null;
  /** Relays a chosen option number (App wires this to wsSend + error). */
  onChoose: (n: number) => void;
}

export function OptionBar({ question, onChoose }: OptionBarProps) {
  const questionID = useId();
  const choices = (question && Array.isArray(question.choices) ? question.choices : []).filter(
    (choice) => typeof choice.number === "number",
  );

  const onChoiceKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    const keys = ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"];
    if (!keys.includes(event.key)) return;

    const buttons = Array.from(
      event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>("button") ?? [],
    );
    const current = buttons.indexOf(event.currentTarget);
    if (current < 0 || buttons.length === 0) return;

    let next = current;
    if (event.key === "Home") next = 0;
    else if (event.key === "End") next = buttons.length - 1;
    else if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
      next = (current - 1 + buttons.length) % buttons.length;
    } else {
      next = (current + 1) % buttons.length;
    }

    event.preventDefault();
    buttons[next]?.focus();
  };

  return (
    <div
      className="option-bar"
      id="option-bar"
      role={choices.length > 0 ? "group" : undefined}
      aria-labelledby={choices.length > 0 ? questionID : undefined}
    >
      {choices.length > 0 ? (
        <span className="option-bar-question" id={questionID}>
          {question?.prompt?.trim() || "Detected prompt choices"}
        </span>
      ) : null}
      {choices.map((c, i) => {
        const num = c.number;
        return (
          <button
            // number is the natural identity; fall back to index for the rare
            // duplicate-number case so React keys stay unique.
            key={"opt-" + num + "-" + i}
            type="button"
            title={c.label || "Option " + num}
            aria-label={c.label ? `Option ${num}: ${c.label}` : `Option ${num}`}
            onPointerDown={onPointerDownNoBlur}
            onKeyDown={onChoiceKeyDown}
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
