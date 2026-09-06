import { defineConfig } from "vite";

// A sandboxed preload script must be CommonJS, and needs the .cjs extension
// for the same reason the main bundle does.
export default defineConfig({
  build: {
    rollupOptions: {
      external: ["electron"],
      output: { format: "cjs", entryFileNames: "[name].cjs" },
    },
  },
});
