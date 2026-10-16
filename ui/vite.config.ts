import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

// The build lands inside internal/web, where the Go binary embeds it, so
// `agentry serve` stays one process. `npm run dev` proxies the API to a
// running `agentry serve` on its default address.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  // Served from this machine, not over the internet, so large bundles are
  // fine: the app is one, and Monaco (VS Code's editor) is a second, loaded
  // only by the pages that show a file.
  build: { outDir: "../internal/web/dist", emptyOutDir: true, chunkSizeWarningLimit: 2500 },
  server: {
    proxy: { "/api": { target: "http://127.0.0.1:7800", ws: true } },
  },
});
