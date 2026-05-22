// Image paste + file upload: ship a clipboard image or picked file to the
// statusd server, which saves it and returns a path. The path is the only
// handoff a terminal pane can take, so callers receive it and decide where
// to drop it (draft textarea or direct-mode stream).

import { $ } from "./dom.js";
import { state, upload } from "./state.js";
import { uploadClipboardImage, uploadPaneFiles } from "./api.js";

export function createUpload({ setInputStatus, showInputError, syncDraft, wsSend }) {
  // clipboardImage returns the first image File on a paste event's clipboard,
  // or null. A screenshot or copied picture arrives as a file item while plain
  // text does not, so this also tells an image paste apart from a text paste.
  function clipboardImage(e) {
    const items = (e.clipboardData && e.clipboardData.items) || [];
    for (const it of items) {
      if (it.kind === "file" && it.type && it.type.indexOf("image/") === 0) {
        const f = it.getAsFile();
        if (f) return f;
      }
    }
    return null;
  }

  let imgUploading = false;

  // pasteImage uploads a clipboard image to the server, which saves it to a
  // file and returns the path; place() then relays that path onward. A terminal
  // pane has no channel for raw image bytes, so the path is the handoff —
  // every supported agent reads an image when given its path.
  async function pasteImage(file, place) {
    if (imgUploading) return; // one upload at a time — ignore a paste mid-flight
    imgUploading = true;
    setInputStatus("uploading image…");
    try {
      const form = new FormData();
      form.append("image", file, file.name || "paste.png");
      const { res, data } = await uploadClipboardImage(form);
      if (!res.ok || !data.path) {
        throw new Error(data.error || ("image upload failed: HTTP " + res.status));
      }
      setInputStatus("");
      place(data.path);
    } catch (e) {
      showInputError(e.message || "image upload failed");
    } finally {
      imgUploading = false;
    }
  }

  async function uploadFilesToPane(files) {
    files = Array.from(files || []).filter(Boolean);
    if (upload.busy || files.length === 0) return;
    if (!state.selected) {
      showInputError("select a pane first");
      return;
    }

    upload.busy = true;
    $("upload-btn").disabled = true;
    setInputStatus(files.length === 1 ? "uploading file…" : "uploading " + files.length + " files…");
    try {
      const form = new FormData();
      files.forEach((file, i) => form.append("file", file, file.name || ("upload-" + (i + 1))));
      const { res, data } = await uploadPaneFiles(form);
      const paths = Array.isArray(data.paths) ? data.paths : (data.path ? [data.path] : []);
      if (!res.ok || paths.length === 0) {
        throw new Error(data.error || ("file upload failed: HTTP " + res.status));
      }
      setInputStatus("");
      if (!wsSend({ t: "text", s: paths.join(" ") + " " })) {
        showInputError("uploaded, but pane is not connected");
      }
    } catch (e) {
      showInputError(e.message || "file upload failed");
    } finally {
      upload.busy = false;
      $("upload-btn").disabled = !state.selected;
    }
  }

  function openFileUploadPicker() {
    const input = $("file-upload");
    input.value = "";
    try {
      if (input.showPicker) input.showPicker();
      else input.click();
    } catch (e) {
      try { input.click(); }
      catch (err) { showInputError(err.message || "file picker blocked"); }
    }
  }

  // placeInDraft inserts text into the draft box at the cursor, or appends it
  // when the draft is unfocused — used to drop a pasted image's path in for
  // review before the prompt is sent.
  function placeInDraft(text) {
    const draft = $("draft");
    if (draft.disabled) return;
    if (document.activeElement === draft &&
        typeof draft.selectionStart === "number") {
      const s = draft.selectionStart, end = draft.selectionEnd;
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
  }

  return {
    clipboardImage,
    pasteImage,
    uploadFilesToPane,
    openFileUploadPicker,
    placeInDraft,
  };
}
