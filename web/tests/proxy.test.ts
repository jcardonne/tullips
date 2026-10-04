import { test } from "node:test";
import assert from "node:assert/strict";
import { publicProxy } from "../src/lib/public-proxy";
test("public MCP proxy forwards security context and does not forward application cookies or impersonation headers", async () => {
  const original = globalThis.fetch;
  globalThis.fetch = async (input, init) => {
    assert.equal(String(input), "http://localhost:8080/mcp");
    const h = new Headers(init?.headers);
    assert.equal(h.get("origin"), "https://untrusted.example");
    assert.equal(h.get("authorization"), "Bearer test-token");
    assert.equal(h.get("cookie"), null);
    assert.equal(h.get("x-tullips-user"), null);
    assert.equal(new TextDecoder().decode(init?.body as ArrayBuffer), "{}");
    return new Response("{}", {
      headers: {
        "content-type": "application/json",
        "set-cookie": "private=value",
      },
    });
  };
  try {
    const response = await publicProxy({
      request: new Request("http://localhost:3000/mcp", {
        method: "POST",
        headers: {
          origin: "https://untrusted.example",
          authorization: "Bearer test-token",
          cookie: "session=private",
          "x-tullips-user": "spoofed",
        },
        body: "{}",
      }),
    });
    assert.equal(response.headers.get("set-cookie"), null);
    assert.equal(response.headers.get("cache-control"), "no-store");
  } finally {
    globalThis.fetch = original;
  }
});

test("oversized chunked bodies are rejected before contacting upstream", async () => {
  const original = globalThis.fetch;
  let forwarded = false;
  globalThis.fetch = async () => {
    forwarded = true;
    return new Response("{}");
  };
  try {
    const response = await publicProxy({
      request: new Request("http://localhost:3000/mcp", {
        method: "POST",
        body: new Uint8Array(1_000_001),
      }),
    });
    assert.equal(response.status, 413);
    assert.equal(forwarded, false);
  } finally {
    globalThis.fetch = original;
  }
});
