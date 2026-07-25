import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const appCss = readFileSync(resolve(process.cwd(), "src/app.css"), "utf8");
const officeCss = readFileSync(
  resolve(process.cwd(), "src/components/OfficeDesks.css"),
  "utf8",
);
const trainCss = readFileSync(
  resolve(process.cwd(), "src/components/TrainLayout.css"),
  "utf8",
);

describe("pane-switcher layout height contract", () => {
  it("defines one desktop/compact scale and derives one shared outer height", () => {
    expect(appCss).toMatch(
      /:root\s*\{[\s\S]*?--pane-switcher-scene-scale:\s*1\.7;[\s\S]*?--pane-switcher-layout-height:\s*calc\(var\(--pane-switcher-scene-scale\) \* 102\.4px \+ 10px\);/,
    );
    expect(appCss).toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?:root\s*\{[\s\S]*?--pane-switcher-scene-scale:\s*1\.36;/,
    );
  });

  it("binds both layout roots to the shared height without duplicating scales", () => {
    expect(officeCss).toMatch(
      /\.office-desks\s*\{[\s\S]*?--ds:\s*var\(--pane-switcher-scene-scale\);[\s\S]*?height:\s*var\(--pane-switcher-layout-height\);[\s\S]*?min-height:\s*var\(--pane-switcher-layout-height\);/,
    );
    expect(trainCss).toMatch(
      /\.train-layout\s*\{[\s\S]*?height:\s*var\(--pane-switcher-layout-height\);/,
    );
    expect(officeCss).not.toMatch(/--ds:\s*(?:1\.7|1\.36);/);
  });

  it("bottom-anchors the unchanged train artwork band below the added sky", () => {
    expect(trainCss).toMatch(
      /\.train-layout\s*\{[\s\S]*?--train-artwork-band-height:\s*clamp\(118px,\s*18vh,\s*160px\);/,
    );
    expect(trainCss).toMatch(
      /\.train-layout-inspection\s*\{[\s\S]*?display:\s*flex;[\s\S]*?align-items:\s*flex-end;/,
    );
    expect(trainCss).toMatch(
      /\.train-layout-scene\s*\{[\s\S]*?height:\s*var\(--train-artwork-band-height\);[\s\S]*?flex:\s*0 0 auto;[\s\S]*?align-items:\s*flex-end;/,
    );
    expect(trainCss).toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?\.train-layout\s*\{[\s\S]*?--train-artwork-band-height:\s*clamp\(104px,\s*16vh,\s*132px\);/,
    );
  });
});
