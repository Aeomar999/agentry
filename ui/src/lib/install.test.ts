import { afterEach, describe, expect, test, vi } from "vitest";

// install.ts keeps its state from import, so each test stubs the browser
// and imports it afresh.
async function loadInstall({ secure, register }: { secure: boolean; register: () => Promise<unknown> }) {
  vi.resetModules();
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {} }));
  vi.stubGlobal("window", { isSecureContext: secure, addEventListener() {} });
  vi.stubGlobal("navigator", { serviceWorker: { register } });
  const install = await import("./install");
  install.watchInstall();
  await new Promise((settle) => setTimeout(settle)); // the registration settles
  return install;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("installState", () => {
  test("is refused on a secure page whose service worker the browser refused", async () => {
    const { installState } = await loadInstall({ secure: true, register: () => Promise.reject(new Error("SSL certificate error")) });
    expect(installState()).toBe("refused");
  });

  test("is unavailable on a secure page with a service worker and no offer yet", async () => {
    const { installState } = await loadInstall({ secure: true, register: () => Promise.resolve({}) });
    expect(installState()).toBe("unavailable");
  });

  test("is insecure on a plain http page, whatever the service worker did", async () => {
    const { installState } = await loadInstall({ secure: false, register: () => Promise.reject(new Error("insecure")) });
    expect(installState()).toBe("insecure");
  });
});
