import { maintenanceResponse } from "../lib/maintenance";
import { boundedBody } from "../lib/request-body";
import { createFileRoute } from "@tanstack/react-router";
import { createHmac } from "node:crypto";
import { auth } from "../lib/auth";
async function proxy({ request }: { request: Request }) {
  if (!new URL(request.url).pathname.startsWith("/api/v1/installation")) {
    const maintenance = await maintenanceResponse();
    if (maintenance) return maintenance;
  }
  const session = await auth.api.getSession({ headers: request.headers });
  if (!session)
    return Response.json(
      { error: "Please sign in to continue." },
      { status: 401 },
    );
  if (
    !["GET", "HEAD"].includes(request.method) &&
    request.headers.get("origin") !==
      new URL(process.env.BETTER_AUTH_URL || request.url).origin
  )
    return Response.json({ error: "Invalid origin" }, { status: 403 });
  const secret = process.env.INTERNAL_API_SECRET;
  if (!secret)
    return Response.json(
      { error: "API proxy is not configured" },
      { status: 503 },
    );
  const url = new URL(request.url);
  const headers = new Headers({
    "X-Tullips-User": session.user.id,
    "X-Tullips-Signature": createHmac("sha256", secret)
      .update(session.user.id)
      .digest("hex"),
  });
  const idem = request.headers.get("idempotency-key");
  if (idem) headers.set("idempotency-key", idem);
  const type = request.headers.get("content-type");
  if (type) headers.set("content-type", type);
  let body: ArrayBuffer | undefined;
  try {
    body = await boundedBody(request, 2000000);
  } catch (e) {
    if (e instanceof RangeError)
      return Response.json({ error: e.message }, { status: 413 });
    throw e;
  }
  const response = await fetch(
    `${process.env.API_URL || "http://localhost:8080"}/api/${url.pathname.replace(/^\/api\/v1\//, "")}${url.search}`,
    {
      method: request.method,
      headers,
      body,
      redirect: "manual",
    },
  );
  return new Response(response.body, {
    status: response.status,
    headers: {
      "content-type":
        response.headers.get("content-type") || "application/json",
      "cache-control": "no-store",
    },
  });
}
export const Route = createFileRoute("/api/v1/$")({
  server: {
    handlers: {
      GET: proxy,
      POST: proxy,
      PATCH: proxy,
      DELETE: proxy,
      PUT: proxy,
    },
  },
});
