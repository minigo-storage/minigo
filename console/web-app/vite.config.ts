import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    // Console's SPA handler replaces this exact token for reverse-proxy mounts.
    {
      name: "console-base-href",
      transformIndexHtml: (html) =>
        html.replace('<base href="/" />', '<base href="/"/>'),
    },
  ],
  base: "./",
  publicDir: false,
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src/minigo", import.meta.url)) },
  },
  build: { outDir: "build", emptyOutDir: true, sourcemap: false },
  server: {
    host: "0.0.0.0",
    proxy: {
      "/api": "http://127.0.0.1:19001",
      "/oauth_callback": "http://127.0.0.1:19001",
    },
  },
});
