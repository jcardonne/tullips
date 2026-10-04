import { boundedBody } from "./request-body";
export async function publicProxy({ request }: { request: Request }) {
  const url = new URL(request.url);
  const headers = new Headers();
  for (const name of [
    "origin",
    "content-type",
    "authorization",
    "mcp-protocol-version",
    "mcp-session-id",
    "accept",
    "last-event-id",
    "stripe-signature",
  ]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  let body: ArrayBuffer | undefined;
  try {
    body = await boundedBody(request, 1000000);
  } catch (e) {
    if (e instanceof RangeError)
      return Response.json({ error: e.message }, { status: 413 });
    throw e;
  }
  const response = await fetch(
    `${process.env.API_URL || "http://localhost:8080"}${url.pathname}${url.search}`,
    {
      method: request.method,
      headers,
      body,
      redirect: "manual",
    },
  );
  const out = new Headers(response.headers);
  out.delete("set-cookie");
  out.set("cache-control", "no-store");
  return new Response(response.body, { status: response.status, headers: out });
}
