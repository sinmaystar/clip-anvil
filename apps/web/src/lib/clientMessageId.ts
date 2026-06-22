export function createClientMessageId() {
  const randomUUID = globalThis.crypto?.randomUUID;
  if (typeof randomUUID === "function") {
    return randomUUID.call(globalThis.crypto);
  }

  const timePart = Date.now().toString(36);
  const randomPart = Math.floor(Math.random() * Number.MAX_SAFE_INTEGER).toString(
    36,
  );
  return `client-${timePart}-${randomPart}`;
}
