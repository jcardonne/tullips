export async function boundedBody(
  request: Request,
  limit: number,
): Promise<ArrayBuffer | undefined> {
  if (["GET", "HEAD"].includes(request.method) || !request.body)
    return undefined;
  if (Number(request.headers.get("content-length")) > limit)
    throw new RangeError("Request body is too large");
  const reader = request.body.getReader(),
    chunks: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > limit) {
        await reader.cancel();
        throw new RangeError("Request body is too large");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const body = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body.buffer;
}
