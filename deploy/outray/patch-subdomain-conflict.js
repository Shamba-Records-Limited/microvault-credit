// Forces takeover on a fresh-process subdomain conflict instead of an interactive
// prompt with no TTY. Throws if outray's source no longer matches this patch.
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");

const clientPath = path.join(
  execSync("npm root -g").toString().trim(),
  "outray",
  "dist",
  "client.js",
);

const target = "this.shouldReconnect = false;\n                        this.handleSubdomainConflict();";
const replacement = "this.forceTakeover = true;\n                        this.connect();";

const source = fs.readFileSync(clientPath, "utf8");
if (!source.includes(target)) {
  throw new Error(`outray subdomain-conflict patch target not found in ${clientPath} — CLI source changed, update the patch`);
}
fs.writeFileSync(clientPath, source.replace(target, replacement));
