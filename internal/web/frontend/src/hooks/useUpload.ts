// Image paste + file upload: ship a clipboard image or picked file to the
// statusd server, which saves it and returns a path. The path is the only
// handoff a terminal pane can take, so callers receive it and decide where
// to drop it (draft textarea or direct-mode stream).
//
// 1:1 behavioral port of static/js/upload.js's createUpload({...}) factory.
// Module-scoped `imgUploading` becomes a ref (preserving the "one upload at a
// time" guard's mutation timing). `state` and `upload` come from the store
// (the original imported them from state.js); the rest are injected deps,
// matching the factory parameters exactly (ARCHITECTURE.md §3).

import { useCallback, useRef } from "react";

import { uploadClipboardImage, uploadPaneFiles } from "../api/client";
import { useAppState } from "../store/AppStateContext";
import type { InputMsg } from "../types/server";

export interface UseUploadDeps {
  /** Persistent (no auto-clear) `#input-error` status text. */
  setInputStatus: (msg: string) => void;
  /** Auto-clearing `#input-error` error text (6000 ms). */
  showInputError: (msg: string) => void;
  /** Re-syncs `#draft-wrap.has-text` + autogrow after a programmatic draft edit. */
  syncDraft: () => void;
  /** Sends an input message over the pane WS; false when not OPEN. */
  wsSend: (msg: InputMsg) => boolean;
  /** Peer string for the `?peer=` query (panePeer(findPane(selected))). */
  getSelectedPeer: () => string;
}

export interface UseUpload {
  clipboardImage: (e: ClipboardEvent) => File | null;
  pasteImage: (file: File, place: (path: string) => void) => Promise<void>;
  uploadFilesToPane: (files: ArrayLike<File> | null | undefined) => Promise<void>;
  openFileUploadPicker: () => void;
  placeInDraft: (text: string) => void;
}

// Read `data.error` defensively: the typed API envelopes only declare
// `path`/`paths`, but the original reads an optional `error` string off the
// parsed body. Mirrors `data.error || (...)` byte-for-behavior.
function dataError(data: unknown): string {
  if (data && typeof data === "object" && "error" in data) {
    const err = (data as { error?: unknown }).error;
    if (typeof err === "string") return err;
  }
  return "";
}

export function useUpload(deps: UseUploadDeps): UseUpload {
  const { setInputStatus, showInputError, syncDraft, wsSend, getSelectedPeer } = deps;
  const { state, upload } = useAppState();

  // Module-scoped `let imgUploading = false` → ref (mutated in place; never
  // re-renders, exactly like the original closure variable).
  const imgUploading = useRef(false);

  // clipboardImage returns the first image File on a paste event's clipboard,
  // or null. A screenshot or copied picture arrives as a file item while plain
  // text does not, so this also tells an image paste apart from a text paste.
  const clipboardImage = useCallback((e: ClipboardEvent): File | null => {
    const items = (e.clipboardData && e.clipboardData.items) || [];
    for (const it of items) {
      if (it.kind === "file" && it.type && it.type.indexOf("image/") === 0) {
        const f = it.getAsFile();
        if (f) return f;
      }
    }
    return null;
  }, []);

  // pasteImage uploads a clipboard image to the server, which saves it to a
  // file and returns the path; place() then relays that path onward. A terminal
  // pane has no channel for raw image bytes, so the path is the handoff —
  // every supported agent reads an image when given its path.
  const pasteImage = useCallback(
    async (file: File, place: (path: string) => void): Promise<void> => {
      if (imgUploading.current) return; // one upload at a time — ignore a paste mid-flight
      imgUploading.current = true;
      setInputStatus("uploading image…");
      try {
        const form = new FormData();
        form.append("image", file, file.name || "paste.png");
        const { res, data } = await uploadClipboardImage(
          form,
          getSelectedPeer ? getSelectedPeer() : undefined,
        );
        if (!res.ok || !data.path) {
          throw new Error(dataError(data) || ("image upload failed: HTTP " + res.status));
        }
        setInputStatus("");
        place(data.path);
      } catch (e) {
        showInputError((e instanceof Error && e.message) || "image upload failed");
      } finally {
        imgUploading.current = false;
      }
    },
    [setInputStatus, showInputError, getSelectedPeer],
  );

  const uploadFilesToPane = useCallback(
    async (input: ArrayLike<File> | null | undefined): Promise<void> => {
      const files = Array.from(input || []).filter(Boolean);
      if (upload.busy || files.length === 0) return;
      if (!state.selected) {
        showInputError("select a pane first");
        return;
      }

      upload.busy = true;
      const uploadBtn = document.getElementById("upload-btn") as HTMLButtonElement | null;
      if (uploadBtn) uploadBtn.disabled = true;
      setInputStatus(
        files.length === 1 ? "uploading file…" : "uploading " + files.length + " files…",
      );
      try {
        const form = new FormData();
        files.forEach((file, i) => form.append("file", file, file.name || ("upload-" + (i + 1))));
        const { res, data } = await uploadPaneFiles(
          form,
          getSelectedPeer ? getSelectedPeer() : undefined,
        );
        const paths = Array.isArray(data.paths) ? data.paths : (data.path ? [data.path] : []);
        if (!res.ok || paths.length === 0) {
          throw new Error(dataError(data) || ("file upload failed: HTTP " + res.status));
        }
        setInputStatus("");
        if (!wsSend({ t: "text", s: paths.join(" ") + " " })) {
          showInputError("uploaded, but pane is not connected");
        }
      } catch (e) {
        showInputError((e instanceof Error && e.message) || "file upload failed");
      } finally {
        upload.busy = false;
        const btn = document.getElementById("upload-btn") as HTMLButtonElement | null;
        if (btn) btn.disabled = !state.selected;
      }
    },
    [upload, state, setInputStatus, showInputError, wsSend, getSelectedPeer],
  );

  const openFileUploadPicker = useCallback((): void => {
    const input = document.getElementById("file-upload") as HTMLInputElement | null;
    if (!input) return;
    input.value = "";
    try {
      if (input.showPicker) input.showPicker();
      else input.click();
    } catch (e) {
      try {
        input.click();
      } catch (err) {
        showInputError((err instanceof Error && err.message) || "file picker blocked");
      }
    }
  }, [showInputError]);

  // placeInDraft inserts text into the draft box at the cursor, or appends it
  // when the draft is unfocused — used to drop a pasted image's path in for
  // review before the prompt is sent.
  const placeInDraft = useCallback(
    (text: string): void => {
      const draft = document.getElementById("draft") as HTMLTextAreaElement | null;
      if (!draft) return;
      if (draft.disabled) return;
      if (
        document.activeElement === draft &&
        typeof draft.selectionStart === "number"
      ) {
        const s = draft.selectionStart;
        const end = draft.selectionEnd;
        draft.value = draft.value.slice(0, s) + text + draft.value.slice(end);
        const pos = s + text.length;
        draft.setSelectionRange(pos, pos);
      } else if (draft.value.trim() === "") {
        draft.value = text;
      } else {
        draft.value = draft.value.replace(/\s*$/, "") + " " + text;
      }
      if (state.selected) state.drafts[state.selected] = draft.value;
      syncDraft();
      draft.focus();
    },
    [state, syncDraft],
  );

  return {
    clipboardImage,
    pasteImage,
    uploadFilesToPane,
    openFileUploadPicker,
    placeInDraft,
  };
}
