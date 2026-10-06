// patchPaneHTML applies a freshly rendered pane HTML string to pre#content by
// replacing only the top-level nodes that changed, instead of reassigning
// innerHTML wholesale.
//
// A working agent repaints its spinner / elapsed timer every capture (200ms),
// so each live frame usually differs from the last in one or two short runs.
// Rewriting innerHTML threw away and re-laid-out the whole pane every time —
// with three split slots streaming that kept a MacBook's renderer and GPU busy
// full time. Here the new HTML is parsed into a detached <template>, its
// top-level nodes are compared against the previous frame's by serialized
// form, and only the differing middle run is swapped, so layout and paint are
// invalidated just around the change.
//
// render() output is not line-local (SGR spans, links and tables can cross
// newlines), which is why the diff unit is a top-level DOM node rather than a
// text line. markPreviewablePaths rewrites text nodes in place, so the live
// DOM is not a 1:1 copy of the template: each template node maps to the
// "group" of live nodes it produced, and comparisons use the template keys.

// ALIGN_RUN consecutive equal keys anchor a scrolled frame to the previous
// one; ALIGN_SEARCH bounds how many leading new nodes are tried as the anchor.
const ALIGN_RUN = 3;
const ALIGN_SEARCH = 32;

export interface PaneDOMState {
  keys: string[];
  groups: Node[][];
}

function nodeKey(n: Node): string {
  if (n.nodeType === 3) return "t" + (n.nodeValue ?? "");
  if (n.nodeType === 1) return "e" + (n as Element).outerHTML;
  return "o" + n.nodeType + (n.nodeValue ?? "");
}

// liveMatches reports whether pre still holds exactly the nodes recorded in
// prev — anything else (placeholder, markdown render, external mutation)
// forces a full rebuild.
function liveMatches(pre: HTMLElement, prev: PaneDOMState): boolean {
  let i = 0;
  const kids = pre.childNodes;
  for (const g of prev.groups) {
    for (const n of g) {
      if (kids[i] !== n) return false;
      i++;
    }
  }
  return i === kids.length;
}

export function patchPaneHTML(
  pre: HTMLElement,
  html: string,
  prev: PaneDOMState | null,
  mark: (root: Node) => void,
): PaneDOMState {
  const doc = pre.ownerDocument;
  const tpl = doc.createElement("template");
  tpl.innerHTML = html;
  const nodes = Array.from(tpl.content.childNodes);
  const keys = nodes.map(nodeKey);

  let oldKeys: string[] = [];
  let oldGroups: Node[][] = [];
  if (prev && liveMatches(pre, prev)) {
    oldKeys = prev.keys;
    oldGroups = prev.groups;
  } else {
    pre.textContent = "";
  }

  // Head alignment. When the pane scrolls, the shown line window slides: the
  // first old node(s) are evicted and everything after shifts up, so a plain
  // common-prefix match finds nothing. Look for the new head inside the old
  // nodes (a run of ALIGN_RUN equal keys, so a recurring "\n" node can't
  // anchor alone) and treat old[0..oldHead) → new[0..newHead) as the replaced
  // head instead.
  let newHead = 0;
  let oldHead = 0;
  if (keys.length && oldKeys.length && keys[0] !== oldKeys[0]) {
    const firstIdx = new Map<string, number>();
    oldKeys.forEach((k, i) => {
      if (!firstIdx.has(k)) firstIdx.set(k, i);
    });
    const limit = Math.min(keys.length, ALIGN_SEARCH);
    for (let a = 0; a < limit; a++) {
      const j = firstIdx.get(keys[a]!);
      if (j === undefined) continue;
      let r = 0;
      while (r < ALIGN_RUN && a + r < keys.length && j + r < oldKeys.length && keys[a + r] === oldKeys[j + r]) r++;
      if (r === ALIGN_RUN || a + r === keys.length || j + r === oldKeys.length) {
        newHead = a;
        oldHead = j;
        break;
      }
    }
  }

  let start = 0;
  const max = Math.min(keys.length - newHead, oldKeys.length - oldHead);
  while (start < max && keys[newHead + start] === oldKeys[oldHead + start]) start++;
  let endNew = keys.length;
  let endOld = oldKeys.length;
  while (
    endNew > newHead + start &&
    endOld > oldHead + start &&
    keys[endNew - 1] === oldKeys[endOld - 1]
  ) {
    endNew--;
    endOld--;
  }

  const firstLive = (from: number): Node | null => {
    for (let i = from; i < oldGroups.length; i++) {
      const n = oldGroups[i]?.[0];
      if (n) return n;
    }
    return null;
  };
  const replace = (fromNew: number, toNew: number, fromOld: number, toOld: number): Node[][] => {
    const anchor = firstLive(toOld);
    for (let i = fromOld; i < toOld; i++) {
      for (const n of oldGroups[i] ?? []) pre.removeChild(n);
    }
    const out: Node[][] = [];
    for (let i = fromNew; i < toNew; i++) {
      const frag = doc.createDocumentFragment();
      frag.appendChild(nodes[i]!);
      mark(frag);
      out.push(Array.from(frag.childNodes));
      pre.insertBefore(frag, anchor);
    }
    return out;
  };

  // Middle first: its anchor (first suffix node) is unaffected by head edits.
  const mid = replace(newHead + start, endNew, oldHead + start, endOld);
  const head = replace(0, newHead, 0, oldHead);

  return {
    keys,
    groups: head.concat(
      oldGroups.slice(oldHead, oldHead + start),
      mid,
      oldGroups.slice(endOld),
    ),
  };
}
