import { createFileRoute } from "@tanstack/react-router";
import { maintenanceResponse } from "../lib/maintenance";
import { auth } from "../lib/auth";
export const Route = createFileRoute("/api/auth/$")({
  server: {
    handlers: {
      GET: async ({ request }) => (await maintenanceResponse()) || auth.handler(request),
      POST: async ({ request }) => (await maintenanceResponse()) || auth.handler(request),
    },
  },
});
