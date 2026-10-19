import { defineConfig, mergeConfig } from "vitest/config";
import { fileURLToPath } from "node:url";
import vite from "./vite.config.ts";

// Tests run in a pretend browser (jsdom), with the app's own aliases. Some
// read the server's Go sources, so the repo root is readable too.
export default mergeConfig(
  vite,
  defineConfig({
    server: { fs: { allow: [fileURLToPath(new URL("..", import.meta.url))] } },
    test: { environment: "jsdom", setupFiles: ["./src/test-setup.ts"] },
  }),
);
