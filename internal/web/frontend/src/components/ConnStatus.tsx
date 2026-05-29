// ConnStatus — the live-connection strip (#conn-status), ported 1:1 from
// app.js setConnStatus (MIGRATION_SPEC §6 item 16; §6 item 83).
//
// app.js:
//   function setConnStatus(msg) {
//     const el = $("conn-status");
//     el.textContent = msg;
//     el.classList.toggle("show", msg !== "");
//   }
//
// The strip lives ABOVE the chips (in index.html order), so showing/hiding it
// reflows only the pane output, never the chip row — preserved here because the
// element is a static sibling of <StatusLine> and only its text/`.show` class
// change. CSS: `.conn-status { display: none }`, `.conn-status.show { display: block }`.
//
// App owns the connection-status string: usePaneStream's onStatus drives App's
// setConnStatus callback, which stores the string (ref + bump) and passes it
// here as `text`. Empty string → no `.show` class (hidden). The string content
// ("connecting…"/"reconnecting…"/"") is set verbatim by App, not by this view.

interface ConnStatusProps {
  /** The current connection-status message; "" hides the strip. */
  text: string;
}

export function ConnStatus({ text }: ConnStatusProps) {
  return (
    <div
      className={"conn-status" + (text !== "" ? " show" : "")}
      id="conn-status"
    >
      {text}
    </div>
  );
}
