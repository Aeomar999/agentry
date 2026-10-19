import { defineConfig, mergeConfig } from "vitest/config";
import vite from "./vite.config.ts";

// Tests run in a pretend browser (jsdom), with the app's own aliases.
export default mergeConfig(
  vite,
  defineConfig({
    test: { environment: "jsdom", setupFiles: ["./src/test-setup.ts"] },
  }),
);
