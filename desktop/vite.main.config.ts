import { defineConfig } from "vite";

/**
 * The main process runs in Electron's Node environment. This workspace is an ES
 * module package, so the CommonJS bundle Electron loads must carry the .cjs
 * extension to avoid being parsed as ESM.
 */
export default defineConfig({
  build: {
    rollupOptions: {
      external: ["electron"],
      output: { format: "cjs", entryFileNames: "[name].cjs" },
    },
  },
});
