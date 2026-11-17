import assert from "node:assert/strict";
import test from "node:test";
import picomatch from "picomatch";

test("the build accepts normal globs and rejects repeated extglobs before compiling them", () => {
  assert.equal(picomatch("**/*.{ts,tsx}")("src/minigo/App.tsx"), true);
  for (const pattern of ["src/+(a)/index.ts", "src/*(a)/index.ts"]) {
    assert.throws(() => picomatch(pattern), /Repeated extglobs are disabled/);
  }
});
