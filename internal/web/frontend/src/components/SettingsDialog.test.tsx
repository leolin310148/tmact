import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  SettingsRefs,
  UseSettingsResult,
} from "../hooks/useSettings";
import SettingsDialog from "./SettingsDialog";

vi.mock("../hooks/usePushNotifications", () => ({
  usePushNotifications: () => ({
    state: "unsubscribed",
    message: "Notifications are available.",
    enable: vi.fn(),
    disable: vi.fn(),
  }),
}));

afterEach(cleanup);

const emptyRefs = (): SettingsRefs => ({
  fontRange: null,
  fontVal: null,
  runningEffect: null,
  paneSwitcherLayout: null,
  voiceDevice: null,
  voiceDeviceStatus: null,
  sttModel: null,
  sttEndpoint: null,
  sttKey: null,
  sttNote: null,
  sttStatus: null,
  sttSave: null,
  frontendBuild: null,
  buildTime: null,
  assetHash: null,
});

function Harness({ asyncStatus = "" }: { asyncStatus?: string }) {
  const [visible, setVisible] = useState(false);
  const refs = useRef<SettingsRefs>(emptyRefs());
  const settings: UseSettingsResult = {
    visible,
    paneSwitcherLayout: "bottom",
    loadClientSettings: vi.fn(),
    openSettings: () => setVisible(true),
    closeSettings: () => setVisible(false),
    refs,
    onFontInput: vi.fn(),
    onFontDec: vi.fn(),
    onFontInc: vi.fn(),
    onRunningEffectChange: vi.fn(),
    onPaneSwitcherLayoutChange: vi.fn(),
    onVoiceDeviceChange: vi.fn(),
    onRefreshVoiceDevices: vi.fn(),
    onSaveSTT: vi.fn(),
    voiceDevices: [],
    selectedVoiceDeviceId: "",
    voiceDeviceStatus: asyncStatus,
    syncFormFromSettings: vi.fn(),
  };

  return (
    <>
      <button type="button" onClick={settings.openSettings}>
        Open settings
      </button>
      <SettingsDialog settings={settings} />
    </>
  );
}

describe("SettingsDialog focus management", () => {
  it("focuses a stable control on open and traps forward and reverse Tab", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(screen.getByRole("button", { name: "Open settings" }));

    const close = screen.getByRole("button", { name: "close settings" });
    const save = screen.getByRole("button", { name: "Save server config" });
    expect(close).toHaveFocus();

    await user.tab({ shift: true });
    expect(save).toHaveFocus();

    await user.tab();
    expect(close).toHaveFocus();
  });

  it("restores the invoking control after button, backdrop, and Escape closes", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const invoker = screen.getByRole("button", { name: "Open settings" });

    await user.click(invoker);
    await user.click(screen.getByRole("button", { name: "close settings" }));
    expect(invoker).toHaveFocus();

    await user.click(invoker);
    fireEvent.mouseDown(document.getElementById("settings-overlay")!);
    expect(invoker).toHaveFocus();

    await user.click(invoker);
    await user.keyboard("{Escape}");
    expect(invoker).toHaveFocus();
  });

  it("does not steal focus when async settings state re-renders the open dialog", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<Harness />);

    await user.click(screen.getByRole("button", { name: "Open settings" }));
    const model = screen.getByRole("textbox", {
      name: "Voice transcription — model",
    });
    model.focus();

    rerender(<Harness asyncStatus="Microphones loaded." />);

    expect(model).toHaveFocus();
  });
});
