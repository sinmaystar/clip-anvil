import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createClientMessageId } from "../../dist-test/lib/clientMessageId.js";

describe("client message id", () => {
  it("creates a fallback id when randomUUID is unavailable", () => {
    const originalCrypto = globalThis.crypto;
    Object.defineProperty(globalThis, "crypto", {
      configurable: true,
      value: {},
    });

    try {
      const id = createClientMessageId();

      assert.match(id, /^client-[0-9a-z]+-[0-9a-z]+$/);
    } finally {
      Object.defineProperty(globalThis, "crypto", {
        configurable: true,
        value: originalCrypto,
      });
    }
  });
});
