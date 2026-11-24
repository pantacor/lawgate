// Copies the repository's LICENSE next to the manifest, so the package
// carries the licence its manifest names. The copy is not committed.
const fs = require("fs");
const path = require("path");

const source = path.join(__dirname, "..", "..", "..", "LICENSE");
const target = path.join(__dirname, "..", "LICENSE");
fs.copyFileSync(source, target);
