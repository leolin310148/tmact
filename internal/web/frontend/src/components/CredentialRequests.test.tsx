import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { declineCredRequest, loadCredRequest, submitCredRequest } from "../api/client";
import { normalizeCredID } from "../lib/credIntent";
import type { CredRequest } from "../types/server";
import CredentialRequests from "./CredentialRequests";

vi.mock("../api/client", () => ({
  loadCredRequest: vi.fn(),
  submitCredRequest: vi.fn(),
  declineCredRequest: vi.fn(),
}));

const mockedLoad = vi.mocked(loadCredRequest);
const mockedSubmit = vi.mocked(submitCredRequest);
const mockedDecline = vi.mocked(declineCredRequest);

const ID = "mini@c_abcdefghijklmnopqrstuvwxyz";

function req(over: Partial<CredRequest> = {}): CredRequest {
  return {
    id: ID,
    name: "github-pat",
    reason: "push release tag",
    host: "mini.local",
    peer: "mini",
    cwd: "/work/app",
    pane_id: "%7",
    created_at: "2026-10-16T00:00:00Z",
    expires_at: new Date(Date.now() + 5 * 60_000).toISOString(),
    state: "pending",
    ...over,
  };
}

function reply(status: number, data: object) {
  return Promise.resolve({ res: { ok: status < 300, status } as Response, data });
}

beforeEach(() => {
  mockedLoad.mockImplementation((id) => reply(200, req({ id })));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  window.history.replaceState(null, "", "/");
});

describe("normalizeCredID", () => {
  it("accepts local and peer ids and rejects anything else", () => {
    expect(normalizeCredID("c_abcdefghijklmnopqrstuvwxyz")).toBe("c_abcdefghijklmnopqrstuvwxyz");
    expect(normalizeCredID("mini%40c_abcdefghijklmnopqrstuvwxyz")).toBe(ID);
    for (const bad of ["", "c_short", "../c_abcdefghijklmnopqrstuvwxyz", "c_ABCDEFGHIJKLMNOPQRSTUVWXYZ", 7]) {
      expect(normalizeCredID(bad)).toBe("");
    }
  });
});

describe("CredentialRequests", () => {
  it("shows a badge for pending requests and opens the oldest", async () => {
    const newer = req({ id: "c_bbbbbbbbbbbbbbbbbbbbbbbbbb", name: "npm-token", created_at: "2026-10-16T00:05:00Z" });
    render(<CredentialRequests pending={[newer, req()]} />);
    const badge = screen.getByRole("button", { name: "2 個待輸入的 credential 請求" });
    fireEvent.click(badge);
    expect(await screen.findByRole("dialog", { name: "Credential: github-pat" })).toBeInTheDocument();
    expect(mockedLoad).toHaveBeenCalledWith(ID);
    expect(screen.getByText("push release tag")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "npm-token @ mini" })).toBeInTheDocument();
    expect(document.getElementById("cred-badge")).toBeNull();
  });

  it("opens from the ?cred= deep link and drops the param", async () => {
    window.history.replaceState(null, "", "/?pane=%257&cred=" + encodeURIComponent(ID));
    render(<CredentialRequests pending={[]} />);
    expect(await screen.findByRole("dialog", { name: "Credential: github-pat" })).toBeInTheDocument();
    expect(window.location.search).toBe("?pane=%257");
  });

  it("submits the value once and clears the input", async () => {
    mockedSubmit.mockImplementation(() => reply(200, req({ state: "submitted", path: "/x" })));
    render(<CredentialRequests pending={[req()]} />);
    fireEvent.click(screen.getByRole("button", { name: /credential 請求/ }));
    await screen.findByRole("dialog");
    const input = document.getElementById("cred-value") as HTMLInputElement;
    expect(input.type).toBe("password");
    fireEvent.click(screen.getByRole("button", { name: "送出" }));
    expect(await screen.findByText("請先輸入值。")).toBeInTheDocument();
    expect(mockedSubmit).not.toHaveBeenCalled();

    input.value = "s3cret";
    fireEvent.click(screen.getByRole("button", { name: "送出" }));
    expect(await screen.findByText("已送出，agent 會收到檔案路徑。")).toBeInTheDocument();
    expect(mockedSubmit).toHaveBeenCalledWith(ID, "s3cret");
    expect(input.value).toBe("");
    expect(input).toBeDisabled();
  });

  it("reports an already-resolved request on submit and on open", async () => {
    mockedSubmit.mockImplementation(() => reply(409, { error: "credential request already resolved" }));
    render(<CredentialRequests pending={[req()]} />);
    fireEvent.click(screen.getByRole("button", { name: /credential 請求/ }));
    await screen.findByRole("dialog");
    (document.getElementById("cred-value") as HTMLInputElement).value = "late";
    fireEvent.click(screen.getByRole("button", { name: "送出" }));
    expect(await screen.findByText("這個請求已經處理過或逾時了。")).toBeInTheDocument();
    cleanup();

    mockedLoad.mockImplementation((id) => reply(200, req({ id, state: "expired" })));
    render(<CredentialRequests pending={[req()]} />);
    fireEvent.click(screen.getByRole("button", { name: /credential 請求/ }));
    expect(await screen.findByText("這個請求已經逾時。")).toBeInTheDocument();
    expect(document.getElementById("cred-value")).toBeDisabled();
  });

  it("declines without sending a value", async () => {
    mockedDecline.mockImplementation(() => reply(200, req({ state: "declined" })));
    render(<CredentialRequests pending={[req()]} />);
    fireEvent.click(screen.getByRole("button", { name: /credential 請求/ }));
    await screen.findByRole("dialog");
    fireEvent.click(screen.getByRole("button", { name: "拒絕" }));
    expect(await screen.findByText("已拒絕。")).toBeInTheDocument();
    expect(mockedDecline).toHaveBeenCalledWith(ID);
    expect(mockedSubmit).not.toHaveBeenCalled();
  });

  it("keeps keystrokes and focus out of the rest of the page while open", async () => {
    const outsideKey = vi.fn();
    const outsideInput = vi.fn();
    render(
      <>
        <textarea id="direct-input" onKeyDown={outsideKey} onInput={outsideInput} />
        <CredentialRequests pending={[req()]} />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: /credential 請求/ }));
    await screen.findByRole("dialog");
    const direct = document.getElementById("direct-input") as HTMLTextAreaElement;
    const value = document.getElementById("cred-value") as HTMLInputElement;

    direct.focus();
    await waitFor(() => expect(document.activeElement).toBe(value));
    fireEvent.keyDown(direct, { key: "a" });
    fireEvent.input(direct, { target: { value: "a" } });
    expect(outsideKey).not.toHaveBeenCalled();
    expect(outsideInput).not.toHaveBeenCalled();

    fireEvent.keyDown(value, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.keyDown(direct, { key: "b" });
    expect(outsideKey).toHaveBeenCalledTimes(1);
  });
});
