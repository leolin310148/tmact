import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import ImagePreview from "./ImagePreview";

afterEach(cleanup);

const firstSrc = "/api/image?path=first.png";

function ClosablePreview() {
  const [src, setSrc] = useState<string | null>(firstSrc);
  return (
    <ImagePreview
      src={src}
      downloadHref="/api/image?path=first.png&download=1"
      path="/work/first.png"
      onClose={() => setSrc(null)}
    />
  );
}

describe("ImagePreview", () => {
  it("shows an explicit loading state until the image loads", () => {
    render(<ImagePreview src={firstSrc} path="first.png" onClose={() => {}} />);

    const image = screen.getByAltText("preview");
    expect(screen.getByRole("status")).toHaveTextContent("Loading image…");
    expect(image).toHaveAttribute("hidden");

    fireEvent.load(image);

    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(image).not.toHaveAttribute("hidden");
  });

  it("replaces a broken image with an error while retaining path and download", () => {
    render(
      <ImagePreview
        src={firstSrc}
        downloadHref="/api/image?path=first.png&download=1"
        path="/work/first.png"
        onClose={() => {}}
      />,
    );

    fireEvent.error(screen.getByAltText("preview"));

    expect(screen.getByRole("alert")).toHaveTextContent("Unable to load image.");
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "download image" })).toHaveAttribute(
      "href",
      "/api/image?path=first.png&download=1",
    );
    expect(document.querySelector(".image-preview-path")).toHaveTextContent("/work/first.png");
  });

  it("starts a fresh guarded load when retrying", async () => {
    const user = userEvent.setup();
    render(<ImagePreview src={firstSrc} path="first.png" onClose={() => {}} />);
    const failedImage = screen.getByAltText("preview");
    fireEvent.error(failedImage);

    await user.click(screen.getByRole("button", { name: "Retry" }));

    const retryImage = screen.getByAltText("preview");
    expect(retryImage).not.toBe(failedImage);
    expect(screen.getByRole("status")).toHaveTextContent("Loading image…");

    fireEvent.load(failedImage);
    expect(screen.getByRole("status")).toBeInTheDocument();

    fireEvent.load(retryImage);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(retryImage).not.toHaveAttribute("hidden");
  });

  it("closes the preview and removes the active image source", async () => {
    const user = userEvent.setup();
    render(<ClosablePreview />);

    await user.click(screen.getByRole("button", { name: "close image preview" }));

    expect(document.querySelector(".image-preview")).toHaveAttribute("hidden");
    expect(screen.getByAltText("preview")).not.toHaveAttribute("src");
    expect(screen.queryByRole("link", { name: "download image" })).not.toBeInTheDocument();
  });

  it("ignores stale load and error events after rapid source changes", () => {
    const { rerender } = render(
      <ImagePreview src={firstSrc} path="first.png" onClose={() => {}} />,
    );
    const firstImage = screen.getByAltText("preview");

    rerender(
      <ImagePreview src="/api/image?path=second.png" path="second.png" onClose={() => {}} />,
    );
    const secondImage = screen.getByAltText("preview");

    fireEvent.load(firstImage);
    fireEvent.error(firstImage);

    expect(screen.getByRole("status")).toHaveTextContent("Loading image…");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    fireEvent.load(secondImage);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(secondImage).not.toHaveAttribute("hidden");
  });
});
