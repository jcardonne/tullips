import { createFileRoute } from "@tanstack/react-router";
import { publicProxy } from "../lib/public-proxy";
export const Route = createFileRoute("/oauth/mail/callback")({
  server: { handlers: { GET: publicProxy } },
});
