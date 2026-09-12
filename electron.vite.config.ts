import { resolve } from "node:path";
import { defineConfig, externalizeDepsPlugin } from "electron-vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  main: {
    plugins: [externalizeDepsPlugin()],
  },
  preload: {
    plugins: [externalizeDepsPlugin()],
  },
  renderer: {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        "@": resolve(__dirname, "src/renderer/src"),
      },
    },
    build: {
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (!id.includes("node_modules")) return;
            if (id.includes("/@xterm/")) return "terminal-vendor";
            if (id.includes("/recharts/") || id.includes("/d3-") || id.includes("/victory-vendor/")) return "charts-vendor";
            if (id.includes("/lucide-react/")) return "icons-vendor";
            if (id.includes("/radix-ui/") || id.includes("/@radix-ui/")) return "radix-vendor";
            return "ui-vendor";
          },
        },
      },
    },
  },
});
