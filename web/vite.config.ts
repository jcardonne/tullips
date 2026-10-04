import { defineConfig } from "vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import react from "@vitejs/plugin-react";
import { nitro } from "nitro/vite";
export default defineConfig({
  plugins: [tanstackStart(), nitro({ preset: "node-server" }), react()],
  server: { port: 3000, host: "0.0.0.0" },
});
