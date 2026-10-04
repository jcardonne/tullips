import { createFileRoute } from "@tanstack/react-router";
import { publicProxy } from "../lib/public-proxy";
export const Route = createFileRoute("/.well-known/$")({
  server: {
    handlers: { GET: publicProxy, POST: publicProxy, DELETE: publicProxy },
  },
});
