// The console build uses ordinary globs. Reject repeated extglobs before regex
// compilation until GHSA-fc5q-39xc-vjpf has an upstream release.
import { createHash } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";

const parser = new URL("../node_modules/picomatch/lib/parse.js", import.meta.url);
const original = "4e1e05447668aa1e94366c5516a2bcd8f5fff800cc92b1d0cd7d77703b8618c4";
const marker = "  const extglobOpen = (type, value) => {";
const guarded = marker + `
    if (type === 'plus' || type === 'star') {
      throw new Error('Repeated extglobs are disabled in the Minigo build');
    }`;
const hash = (s) => createHash("sha256").update(s).digest("hex");

if (existsSync(parser)) {
  const source = readFileSync(parser, "utf8");
  if (source.includes(guarded) && hash(source.replace(guarded, marker)) === original) {
    // Already applied to exactly the reviewed parser.
  } else if (hash(source) === original && !process.argv.includes("--check")) {
    writeFileSync(parser, source.replace(marker, guarded));
  } else {
    throw new Error("Review the picomatch security guard for the installed parser");
  }
} else if (process.argv.includes("--check")) {
  throw new Error("The build dependency picomatch must be installed for verification");
}
