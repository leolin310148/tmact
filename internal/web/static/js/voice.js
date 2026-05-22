// Voice transcription: record locally, transcribe on the statusd server.
// Factory function so app.js can wire in its DOM-touching helpers
// (showInputError, syncDraft) without this module reaching back into app.js.

import { $, isMobile } from "./dom.js";
import { transcribeAudio } from "./api.js";
import { state, voice } from "./state.js";

export function createVoice({ showInputError, syncDraft }) {
  function voiceSupported() {
    return !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia && window.MediaRecorder);
  }

  function preferredAudioType() {
    const types = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4", "audio/wav"];
    for (const t of types) {
      if (MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(t)) return t;
    }
    return "";
  }

  // The record button only ever starts a recording — the live state and the
  // stop control live in the overlay — so it just reflects enabled/busy.
  function syncRecordButton() {
    const btn = $("record-btn");
    btn.classList.toggle("busy", voice.busy);
    btn.disabled = voice.busy || !!voice.pendingBlob || !state.selected || !voiceSupported();
    btn.title = voice.busy ? "transcribing…" : voice.pendingBlob ? "confirm recording" : "record voice";
  }

  function fmtElapsed(ms) {
    const s = Math.floor(ms / 1000);
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  // positionRecOverlay pins the Stop button right over the mic button (same
  // size, same spot) and the info card just above the input bar, so the finger
  // that tapped Record only has to tap again to stop.
  function positionRecOverlay() {
    const ov = $("rec-overlay");
    if (ov.hidden) return;
    const mic = $("record-btn").getBoundingClientRect();
    const stop = $("rec-stop");
    stop.style.left = mic.left + "px";
    stop.style.top = mic.top + "px";
    stop.style.width = mic.width + "px";
    stop.style.height = mic.height + "px";
    const bar = $("input-bar").getBoundingClientRect();
    ov.querySelector(".rec-card").style.bottom =
      Math.max(8, window.innerHeight - bar.top + 8) + "px";
  }

  function showRecOverlay() {
    const ov = $("rec-overlay");
    ov.classList.remove("transcribing", "confirming", "hotkey-recording");
    $("rec-label").textContent = "Recording…";
    $("rec-timer").textContent = "0:00";
    ov.hidden = false;
    positionRecOverlay();
    voice.startedAt = Date.now();
    if (voice.timer) clearInterval(voice.timer);
    voice.timer = setInterval(() => {
      $("rec-timer").textContent = fmtElapsed(Date.now() - voice.startedAt);
    }, 250);
  }

  function showHotkeyRecordingOverlay() {
    const ov = $("rec-overlay");
    ov.classList.remove("transcribing", "confirming");
    ov.classList.add("hotkey-recording");
    $("rec-label").textContent = "Recording…";
    $("rec-timer").textContent = "0:00";
    ov.hidden = false;
    voice.startedAt = Date.now();
    if (voice.timer) clearInterval(voice.timer);
    voice.timer = setInterval(() => {
      $("rec-timer").textContent = fmtElapsed(Date.now() - voice.startedAt);
    }, 250);
  }

  function recOverlayTranscribing() {
    $("rec-overlay").classList.add("transcribing");
    $("rec-overlay").classList.remove("confirming", "hotkey-recording");
    $("rec-label").textContent = "Transcribing…";
    if (voice.timer) { clearInterval(voice.timer); voice.timer = null; }
  }

  function hideRecOverlay() {
    $("rec-overlay").hidden = true;
    $("rec-overlay").classList.remove("transcribing", "confirming", "hotkey-recording");
    if (voice.timer) { clearInterval(voice.timer); voice.timer = null; }
  }

  function showRecordingConfirm(blob) {
    voice.pendingBlob = blob;
    const ov = $("rec-overlay");
    ov.classList.remove("transcribing", "hotkey-recording");
    ov.classList.add("confirming");
    $("rec-label").textContent = "Send recording?";
    $("rec-timer").textContent = "V send · C cancel";
    ov.hidden = false;
    if (voice.timer) { clearInterval(voice.timer); voice.timer = null; }
    syncRecordButton();
  }

  function finishRecordingConfirm(send) {
    const blob = voice.pendingBlob;
    voice.pendingBlob = null;
    syncRecordButton();
    if (send && blob) {
      uploadRecording(blob);
      return;
    }
    hideRecOverlay();
  }

  function insertTranscript(text) {
    const draft = $("draft");
    const clean = text.trim();
    if (!clean) {
      showInputError("empty transcript");
      return;
    }
    if (document.activeElement === draft &&
        typeof draft.selectionStart === "number" &&
        typeof draft.selectionEnd === "number") {
      const start = draft.selectionStart, end = draft.selectionEnd;
      draft.value = draft.value.slice(0, start) + clean + draft.value.slice(end);
      const pos = start + clean.length;
      draft.setSelectionRange(pos, pos);
    } else if (!draft.value.trim()) {
      draft.value = clean;
    } else {
      draft.value = draft.value.replace(/\s*$/, "") + "\n" + clean;
    }
    if (state.selected) state.drafts[state.selected] = draft.value;
    draft.disabled = false;
    syncDraft();
    draft.focus();
  }

  // recordingFilename names the upload after its real container. The server
  // re-sniffs the bytes regardless, but a correct name is a useful fallback.
  function recordingFilename(type) {
    const t = (type || "").toLowerCase();
    let ext = "webm";
    if (t.includes("webm")) ext = "webm";
    else if (t.includes("ogg") || t.includes("oga")) ext = "ogg";
    else if (t.includes("mp4") || t.includes("m4a") || t.includes("aac")) ext = "m4a";
    else if (t.includes("mpeg") || t.includes("mpga") || t.includes("mp3")) ext = "mp3";
    else if (t.includes("wav")) ext = "wav";
    return "recording." + ext;
  }

  async function uploadRecording(blob) {
    if (!blob || blob.size === 0) {
      hideRecOverlay();
      showInputError("no audio recorded");
      return;
    }
    voice.busy = true;
    syncRecordButton();
    recOverlayTranscribing();
    try {
      const form = new FormData();
      form.append("audio", blob, recordingFilename(voice.mimeType || blob.type));
      const { res, data } = await transcribeAudio(form);
      if (!res.ok) throw new Error(data.error || ("transcription failed: HTTP " + res.status));
      insertTranscript(data.text || "");
    } catch (e) {
      showInputError(e.message || "transcription failed");
    } finally {
      voice.busy = false;
      hideRecOverlay();
      syncRecordButton();
    }
  }

  async function startRecording(recordOpts) {
    if (!state.selected || voice.busy || voice.recorder || voice.pendingBlob) return false;
    if (!voiceSupported()) {
      showInputError("microphone recording is not supported in this browser");
      return false;
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const type = preferredAudioType();
      const mediaOpts = type ? { mimeType: type } : undefined;
      const recorder = new MediaRecorder(stream, mediaOpts);
      voice.stream = stream;
      voice.recorder = recorder;
      voice.chunks = [];
      voice.canceled = false;
      voice.confirmOnStop = !!(recordOpts && recordOpts.confirmOnStop === true);
      // Trust the type we asked for: iOS Safari leaves recorder.mimeType empty,
      // which would otherwise mislabel an MP4 recording as webm.
      voice.mimeType = type || recorder.mimeType || "";
      recorder.ondataavailable = (ev) => {
        if (ev.data && ev.data.size > 0) voice.chunks.push(ev.data);
      };
      recorder.onerror = () => {
        hideRecOverlay();
        showInputError("microphone recording failed");
      };
      recorder.onstop = () => {
        stream.getTracks().forEach((track) => track.stop());
        const blob = new Blob(voice.chunks, { type: voice.mimeType || recorder.mimeType || "audio/mp4" });
        const canceled = voice.canceled;
        voice.recorder = null;
        voice.stream = null;
        voice.chunks = [];
        const confirmOnStop = voice.confirmOnStop;
        voice.confirmOnStop = false;
        voice.hotkeyDown = false;
        voice.hotkeyStopPending = false;
        syncRecordButton();
        if (canceled) { hideRecOverlay(); return; }
        if (confirmOnStop) showRecordingConfirm(blob);
        else uploadRecording(blob);
      };
      recorder.start();
      if (voice.confirmOnStop) showHotkeyRecordingOverlay();
      else showRecOverlay();
      syncRecordButton();
      if (voice.hotkeyStopPending) {
        voice.hotkeyStopPending = false;
        stopRecording();
      }
      return true;
    } catch (e) {
      const denied = e && (e.name === "NotAllowedError" || e.name === "PermissionDeniedError");
      showInputError(denied ? "microphone permission denied" : "microphone unavailable");
      if (voice.stream) voice.stream.getTracks().forEach((track) => track.stop());
      voice.stream = null;
      voice.recorder = null;
      voice.confirmOnStop = false;
      voice.hotkeyDown = false;
      voice.hotkeyStopPending = false;
      hideRecOverlay();
      syncRecordButton();
      return false;
    }
  }

  // Button recordings stop straight into transcription. Hotkey recordings use a
  // confirmation step after key release, so an accidental Option+V hold can still
  // be canceled before uploading.
  function stopRecording() {
    if (voice.recorder && voice.recorder.state === "recording") {
      voice.recorder.stop();
    }
  }

  function cancelRecording() {
    if (voice.pendingBlob) {
      finishRecordingConfirm(false);
      return;
    }
    if (voice.recorder && voice.recorder.state === "recording") {
      voice.canceled = true;
      voice.recorder.stop();
    } else {
      voice.confirmOnStop = false;
      hideRecOverlay();
    }
  }

  function isRecordHotkey(e) {
    return e.altKey && !e.ctrlKey && !e.metaKey && e.code === "KeyV";
  }

  function suppressRecordTextInput(e) {
    voice.suppressInputUntil = Date.now() + 700;
    const draft = $("draft");
    if (document.activeElement === draft) voice.suppressedDraftValue = draft.value;
    e.preventDefault();
    e.stopPropagation();
  }

  function isTextInputTarget(el) {
    return el === $("draft") || el === $("direct-input");
  }

  function stopHotkeyRecording() {
    if (!voice.hotkeyDown && !voice.hotkeyStopPending) return;
    voice.hotkeyDown = false;
    voice.suppressInputUntil = Date.now() + 700;
    if (voice.recorder && voice.recorder.state === "recording") {
      voice.hotkeyStopPending = false;
      stopRecording();
    } else {
      voice.hotkeyStopPending = true;
    }
  }

  function wireRecordHotkey() {
    document.addEventListener("keydown", async (e) => {
      if (isMobile() || $("settings-overlay").hidden === false) return;
      if (voice.pendingBlob && (e.code === "KeyV" || e.code === "KeyC" || e.key === "Escape")) {
        e.preventDefault();
        e.stopPropagation();
        finishRecordingConfirm(e.code === "KeyV");
        return;
      }
      if (!isRecordHotkey(e)) return;
      if (e.repeat || voice.hotkeyDown) {
        suppressRecordTextInput(e);
        return;
      }
      suppressRecordTextInput(e);
      voice.hotkeyDown = true;
      voice.hotkeyStopPending = false;
      const started = await startRecording({ confirmOnStop: true });
      if (!started) {
        voice.hotkeyDown = false;
        voice.hotkeyStopPending = false;
      }
    }, true);

    document.addEventListener("keyup", (e) => {
      if (!voice.hotkeyDown && !voice.hotkeyStopPending) return;
      if (e.code !== "KeyV" && e.code !== "AltLeft" && e.code !== "AltRight") return;
      suppressRecordTextInput(e);
      stopHotkeyRecording();
    }, true);

    document.addEventListener("beforeinput", (e) => {
      if (Date.now() > voice.suppressInputUntil || !isTextInputTarget(e.target)) return;
      e.preventDefault();
      e.stopPropagation();
    }, true);

    document.addEventListener("input", (e) => {
      if (Date.now() > voice.suppressInputUntil || !isTextInputTarget(e.target)) return;
      e.preventDefault();
      e.stopPropagation();
      if (e.target === $("draft")) {
        e.target.value = voice.suppressedDraftValue || "";
        if (state.selected) state.drafts[state.selected] = e.target.value;
        syncDraft();
      } else {
        e.target.value = "";
      }
    }, true);

    window.addEventListener("blur", stopHotkeyRecording);
  }

  return {
    syncRecordButton,
    positionRecOverlay,
    startRecording,
    stopRecording,
    cancelRecording,
    finishRecordingConfirm,
    wireRecordHotkey,
  };
}
