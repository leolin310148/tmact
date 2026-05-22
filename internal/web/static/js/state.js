export const state = {
  selected: null,
  snapshot: null,
  drafts: {},
  paneOrder: [],
  selectionMode: false,
};

export const voice = {
  recorder: null,
  stream: null,
  chunks: [],
  busy: false,
  mimeType: "",
  canceled: false,
  timer: null,
  startedAt: 0,
  confirmOnStop: false,
  hotkeyDown: false,
  hotkeyStopPending: false,
  pendingBlob: null,
  suppressInputUntil: 0,
  suppressedDraftValue: null,
};

export const upload = {
  busy: false,
};
