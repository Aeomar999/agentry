import { describe, expect, test } from "vitest";
import { httpsAddress, shortFingerprint } from "./trust";

describe("shortFingerprint", () => {
  test("keeps the first eight pairs of a SHA-256", () => {
    const full = Array.from({ length: 32 }, (_, i) => i.toString(16).padStart(2, "0").toUpperCase()).join(":");
    expect(shortFingerprint(full)).toBe("00:01:02:03:04:05:06:07…");
  });

  test("leaves a short one as it is", () => {
    expect(shortFingerprint("AB:CD")).toBe("AB:CD");
  });
});

describe("httpsAddress", () => {
  test("keeps the host and port", () => {
    expect(httpsAddress({ protocol: "http:", host: "192.168.1.5:7800", port: "7800" })).toBe("https://192.168.1.5:7800");
    expect(httpsAddress({ protocol: "http:", host: "[fd7a::5]:7800", port: "7800" })).toBe("https://[fd7a::5]:7800");
  });

  test("names port 80 for a page open at http's own port", () => {
    expect(httpsAddress({ protocol: "http:", host: "agentry.lan", port: "" })).toBe("https://agentry.lan:80");
  });

  test("leaves an https page as it is", () => {
    expect(httpsAddress({ protocol: "https:", host: "agentry.lan", port: "" })).toBe("https://agentry.lan");
  });
});
