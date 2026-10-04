import { createFileRoute } from "@tanstack/react-router";
import { publicProxy } from "../lib/public-proxy";
export const Route = createFileRoute("/oauth/revoke")({
  server: {
    handlers: { GET: publicProxy, POST: publicProxy, DELETE: publicProxy },
  },
});
