// Voice transcription hook — 1:1 behavioral port of static/js/voice.js
// (the `createVoice({ showInputError, syncDraft })` factory) plus the voice
// wiring that lived in static/app.js (`wireRecordHotkey`, record-button click).
//
// PARITY MODEL (read ARCHITECTURE.md §3, §9 step 5):
//   The original module owned the MediaRecorder/MediaStream/timer handles and
//   the hotkey/suppression bookkeeping in the module-scoped `voice` object,
//   mutated BY REFERENCE. Here that object comes from `useAppState().voice`
//   (a stable ref), mutated in place IDENTICALLY to voice.js — never useState.
//
//   All DOM the original touched ($("rec-overlay"), $("draft"), …) is reached
//   imperatively via getElementById, exactly as the original `$` did, so the
//   overlay state classes / timer text / pinning math match byte-for-behavior.
//   (The RecOverlay component only renders the static markup; useVoice drives
//   it imperatively just like voice.js drove the hand-written HTML.)
//
//   `wireRecordHotkey` adds CAPTURE-phase listeners on document (keydown/keyup/
//   beforeinput/input) plus a window blur listener, registered once on mount so
//   they precede #direct-input's bubble-phase keydown (ARCHITECTURE.md §9 step 5).

import { useCallback, useEffect, useRef } from "react";
import { isMobile } from "../lib/dom";
import { transcribeAudio } from "../api/client";
import { useAppState } from "../store/AppStateContext";

// $ mirrors dom.js's getElementById helper. Voice's DOM is imperative (overlay
// state classes, timer text, draft mutation) exactly as in the original, so we
// reach the live nodes by id rather than through React.
function $(id: string): HTMLElement | null {
  return document.getElementById(id);
}

/**
 * Injected-deps contract — mirrors `createVoice({ showInputError, syncDraft })`
 * in voice.js (see ARCHITECTURE.md §3). App provides these from its callbacks;
 * the hook never reaches back into App by import.
 */
export interface UseVoiceDeps {
  /** app.js `showInputError(msg)` — transient input-bar error (6 s auto-clear). */
  showInputError: (msg: string) => void;
  /** app.js `syncDraft()` — reconcile draft clear-button + textarea height. */
  syncDraft: () => void;
}

/**
 * The object voice.js's factory returned, plus `wireRecordHotkey`. App destructures
 * these and wires them (record-button click → startRecording, rec-stop/send/cancel
 * buttons, viewport positionRecOverlay, capture-phase hotkey listeners).
 */
export interface UseVoice {
  syncRecordButton: () => void;
  positionRecOverlay: () => void;
  startRecording: (recordOpts?: { confirmOnStop?: boolean }) => Promise<boolean>;
  stopRecording: () => void;
  cancelRecording: () => void;
  finishRecordingConfirm: (send: boolean) => void;
  wireRecordHotkey: () => void;
}

export function useVoice({ showInputError, syncDraft }: UseVoiceDeps): UseVoice {
  const { state, voice } = useAppState();

  // Deps are read through a ref so the stable callback identities below always
  // invoke the latest App-provided implementations (the original closed over a
  // single showInputError/syncDraft reference for the module's lifetime).
  const depsRef = useRef<UseVoiceDeps>({ showInputError, syncDraft });
  depsRef.current = { showInputError, syncDraft };

  function voiceSupported(): boolean {
    return !!(
      navigator.mediaDevices &&
      typeof navigator.mediaDevices.getUserMedia === "function" &&
      window.MediaRecorder
    );
  }

  function preferredAudioType(): string {
    const types = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4", "audio/wav"];
    for (const t of types) {
      // iOS Safari leaves MediaRecorder.isTypeSupported undefined — guard it.
      if (MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(t)) return t;
    }
    return "";
  }

  // The record button only ever starts a recording — the live state and the
  // stop control live in the overlay — so it just reflects enabled/busy.
  const syncRecordButton = useCallback((): void => {
    const btn = $("record-btn") as HTMLButtonElement | null;
    if (!btn) return;
    btn.classList.toggle("busy", voice.busy);
    btn.disabled =
      voice.busy || !!voice.pendingBlob || !state.selected || !voiceSupported();
    btn.title = voice.busy
      ? "transcribing…"
      : voice.pendingBlob
        ? "confirm recording"
        : "record voice";
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state, voice]);

  function fmtElapsed(ms: number): string {
    const s = Math.floor(ms / 1000);
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  // positionRecOverlay pins the Stop button right over the mic button (same
  // size, same spot) and the info card just above the input bar, so the finger
  // that tapped Record only has to tap again to stop.
  const positionRecOverlay = useCallback((): void => {
    const ov = $("rec-overlay");
    if (!ov || (ov as HTMLElement & { hidden: boolean }).hidden) return;
    const recordBtn = $("record-btn");
    const stop = $("rec-stop");
    if (!recordBtn || !stop) return;
    const mic = recordBtn.getBoundingClientRect();
    stop.style.left = mic.left + "px";
    stop.style.top = mic.top + "px";
    stop.style.width = mic.width + "px";
    stop.style.height = mic.height + "px";
    const inputBar = $("input-bar");
    if (!inputBar) return;
    const bar = inputBar.getBoundingClientRect();
    const card = ov.querySelector<HTMLElement>(".rec-card");
    if (card) {
      card.style.bottom = Math.max(8, window.innerHeight - bar.top + 8) + "px";
    }
  }, []);

  function showRecOverlay(): void {
    const ov = $("rec-overlay");
    if (!ov) return;
    ov.classList.remove("transcribing", "confirming", "hotkey-recording");
    const label = $("rec-label");
    if (label) label.textContent = "Recording…";
    const timer = $("rec-timer");
    if (timer) timer.textContent = "0:00";
    (ov as HTMLElement & { hidden: boolean }).hidden = false;
    positionRecOverlay();
    voice.startedAt = Date.now();
    if (voice.timer) clearInterval(voice.timer);
    voice.timer = setInterval(() => {
      const t = $("rec-timer");
      if (t) t.textContent = fmtElapsed(Date.now() - voice.startedAt);
    }, 250);
  }

  function showHotkeyRecordingOverlay(): void {
    const ov = $("rec-overlay");
    if (!ov) return;
    ov.classList.remove("transcribing", "confirming");
    ov.classList.add("hotkey-recording");
    const label = $("rec-label");
    if (label) label.textContent = "Recording…";
    const timer = $("rec-timer");
    if (timer) timer.textContent = "0:00";
    (ov as HTMLElement & { hidden: boolean }).hidden = false;
    voice.startedAt = Date.now();
    if (voice.timer) clearInterval(voice.timer);
    voice.timer = setInterval(() => {
      const t = $("rec-timer");
      if (t) t.textContent = fmtElapsed(Date.now() - voice.startedAt);
    }, 250);
  }

  function recOverlayTranscribing(): void {
    const ov = $("rec-overlay");
    if (ov) {
      ov.classList.add("transcribing");
      ov.classList.remove("confirming", "hotkey-recording");
    }
    const label = $("rec-label");
    if (label) label.textContent = "Transcribing…";
    if (voice.timer) {
      clearInterval(voice.timer);
      voice.timer = null;
    }
  }

  function hideRecOverlay(): void {
    const ov = $("rec-overlay");
    if (ov) {
      (ov as HTMLElement & { hidden: boolean }).hidden = true;
      ov.classList.remove("transcribing", "confirming", "hotkey-recording");
    }
    if (voice.timer) {
      clearInterval(voice.timer);
      voice.timer = null;
    }
  }

  function showRecordingConfirm(blob: Blob): void {
    voice.pendingBlob = blob;
    const ov = $("rec-overlay");
    if (ov) {
      ov.classList.remove("transcribing", "hotkey-recording");
      ov.classList.add("confirming");
    }
    const label = $("rec-label");
    if (label) label.textContent = "Send recording?";
    const timer = $("rec-timer");
    if (timer) timer.textContent = "V send · C cancel";
    if (ov) (ov as HTMLElement & { hidden: boolean }).hidden = false;
    if (voice.timer) {
      clearInterval(voice.timer);
      voice.timer = null;
    }
    syncRecordButton();
  }

  const finishRecordingConfirm = useCallback((send: boolean): void => {
    const blob = voice.pendingBlob;
    voice.pendingBlob = null;
    syncRecordButton();
    if (send && blob) {
      uploadRecording(blob);
      return;
    }
    hideRecOverlay();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [voice, syncRecordButton]);

  function insertTranscript(text: string): void {
    const draft = $("draft") as HTMLTextAreaElement | null;
    if (!draft) return;
    const clean = text.trim();
    if (!clean) {
      depsRef.current.showInputError("empty transcript");
      return;
    }
    if (
      document.activeElement === draft &&
      typeof draft.selectionStart === "number" &&
      typeof draft.selectionEnd === "number"
    ) {
      const start = draft.selectionStart;
      const end = draft.selectionEnd;
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
    depsRef.current.syncDraft();
    draft.focus();
  }

  // recordingFilename names the upload after its real container. The server
  // re-sniffs the bytes regardless, but a correct name is a useful fallback.
  function recordingFilename(type: string): string {
    const t = (type || "").toLowerCase();
    let ext = "webm";
    if (t.includes("webm")) ext = "webm";
    else if (t.includes("ogg") || t.includes("oga")) ext = "ogg";
    else if (t.includes("mp4") || t.includes("m4a") || t.includes("aac")) ext = "m4a";
    else if (t.includes("mpeg") || t.includes("mpga") || t.includes("mp3")) ext = "mp3";
    else if (t.includes("wav")) ext = "wav";
    return "recording." + ext;
  }

  async function uploadRecording(blob: Blob): Promise<void> {
    if (!blob || blob.size === 0) {
      hideRecOverlay();
      depsRef.current.showInputError("no audio recorded");
      return;
    }
    voice.busy = true;
    syncRecordButton();
    recOverlayTranscribing();
    try {
      const form = new FormData();
      form.append("audio", blob, recordingFilename(voice.mimeType || blob.type));
      const { res, data } = await transcribeAudio(form);
      // data is typed { text?: string }; the server also returns `error` on
      // failure (read defensively, exactly as voice.js: data.error || …).
      const err = (data as { error?: string }).error;
      if (!res.ok) throw new Error(err || "transcription failed: HTTP " + res.status);
      insertTranscript(data.text || "");
    } catch (e) {
      depsRef.current.showInputError(
        (e instanceof Error && e.message) || "transcription failed",
      );
    } finally {
      voice.busy = false;
      hideRecOverlay();
      syncRecordButton();
    }
  }

  // stopRecording is referenced by startRecording's hotkeyStopPending race and by
  // the hotkey listeners; declared as a stable ref-backed callback below so the
  // forward reference resolves without a stale closure.
  const stopRecording = useCallback((): void => {
    if (voice.recorder && voice.recorder.state === "recording") {
      voice.recorder.stop();
    }
  }, [voice]);

  const startRecording = useCallback(
    async (recordOpts?: { confirmOnStop?: boolean }): Promise<boolean> => {
      if (!state.selected || voice.busy || voice.recorder || voice.pendingBlob) {
        return false;
      }
      if (!voiceSupported()) {
        depsRef.current.showInputError(
          "microphone recording is not supported in this browser",
        );
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
          depsRef.current.showInputError("microphone recording failed");
        };
        recorder.onstop = () => {
          stream.getTracks().forEach((track) => track.stop());
          const blob = new Blob(voice.chunks, {
            type: voice.mimeType || recorder.mimeType || "audio/mp4",
          });
          const canceled = voice.canceled;
          voice.recorder = null;
          voice.stream = null;
          voice.chunks = [];
          const confirmOnStop = voice.confirmOnStop;
          voice.confirmOnStop = false;
          voice.hotkeyDown = false;
          voice.hotkeyStopPending = false;
          syncRecordButton();
          if (canceled) {
            hideRecOverlay();
            return;
          }
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
        const name = e instanceof Error ? e.name : "";
        const denied =
          name === "NotAllowedError" || name === "PermissionDeniedError";
        depsRef.current.showInputError(
          denied ? "microphone permission denied" : "microphone unavailable",
        );
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
      // eslint-disable-next-line react-hooks/exhaustive-deps
    },
    [state, voice, syncRecordButton, stopRecording],
  );

  const cancelRecording = useCallback((): void => {
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [voice, finishRecordingConfirm]);

  function isRecordHotkey(e: KeyboardEvent): boolean {
    return e.altKey && !e.ctrlKey && !e.metaKey && e.code === "KeyV";
  }

  function suppressRecordTextInput(e: Event): void {
    voice.suppressInputUntil = Date.now() + 700;
    const draft = $("draft") as HTMLTextAreaElement | null;
    if (draft && document.activeElement === draft) {
      voice.suppressedDraftValue = draft.value;
    }
    e.preventDefault();
    e.stopPropagation();
  }

  function isTextInputTarget(el: EventTarget | null): boolean {
    return el === $("draft") || el === $("direct-input");
  }

  function stopHotkeyRecording(): void {
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

  // wireRecordHotkey registers the capture-phase document listeners (and the
  // window blur fallback) ONCE on mount, mirroring app.js's top-level call.
  // ARCHITECTURE.md §9 step 5: these must precede #direct-input's keydown, which
  // is bubble-phase — capture always wins. Listeners read live behavior through
  // the latest closures (recreated only on hook mount, like the original).
  const wireRecordHotkey = useCallback((): void => {
    const onKeyDown = async (e: KeyboardEvent): Promise<void> => {
      const settings = $("settings-overlay") as
        | (HTMLElement & { hidden: boolean })
        | null;
      if (isMobile() || (settings && settings.hidden === false)) return;
      if (
        voice.pendingBlob &&
        (e.code === "KeyV" || e.code === "KeyC" || e.key === "Escape")
      ) {
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
    };

    const onKeyUp = (e: KeyboardEvent): void => {
      if (!voice.hotkeyDown && !voice.hotkeyStopPending) return;
      if (e.code !== "KeyV" && e.code !== "AltLeft" && e.code !== "AltRight") return;
      suppressRecordTextInput(e);
      stopHotkeyRecording();
    };

    const onBeforeInput = (e: Event): void => {
      if (Date.now() > voice.suppressInputUntil || !isTextInputTarget(e.target)) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
    };

    const onInput = (e: Event): void => {
      if (Date.now() > voice.suppressInputUntil || !isTextInputTarget(e.target)) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      const target = e.target as (HTMLTextAreaElement | HTMLInputElement) | null;
      if (!target) return;
      if (target === $("draft")) {
        target.value = voice.suppressedDraftValue || "";
        if (state.selected) state.drafts[state.selected] = target.value;
        depsRef.current.syncDraft();
      } else {
        target.value = "";
      }
    };

    document.addEventListener("keydown", onKeyDown, true);
    document.addEventListener("keyup", onKeyUp, true);
    document.addEventListener("beforeinput", onBeforeInput, true);
    document.addEventListener("input", onInput, true);
    window.addEventListener("blur", stopHotkeyRecording);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [voice, state, startRecording, finishRecordingConfirm, stopRecording]);

  // Clean up live recorder/stream/timer on unmount. The original module never
  // unmounted; App owns voice for its whole lifetime. This guards a stray
  // recorder/timer surviving a hot reload without altering runtime semantics.
  useEffect(() => {
    return () => {
      if (voice.timer) {
        clearInterval(voice.timer);
        voice.timer = null;
      }
      if (voice.recorder && voice.recorder.state === "recording") {
        try {
          voice.recorder.stop();
        } catch {
          // ignore
        }
      }
      if (voice.stream) {
        voice.stream.getTracks().forEach((track) => track.stop());
        voice.stream = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
