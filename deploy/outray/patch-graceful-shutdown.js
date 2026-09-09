// client.stop() calls ws.close(), which is async, but the `outray http`
// command's SIGINT/SIGTERM handlers call process.exit(0) on the very next
// line without waiting for the close frame to actually reach the server.
// Under `docker stop`/a compose recreate, the process dies before the server
// sees a clean close, so it keeps the subdomain session alive — the next
// container's connect() then fights the zombie session for the same
// subdomain (SUBDOMAIN_IN_USE loop). Give the close frame a moment to flush
// before exiting. Throws if outray's source no longer matches this patch.
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");

const indexPath = path.join(
  execSync("npm root -g").toString().trim(),
  "outray",
  "dist",
  "index.js",
);

const target = "        client.stop();\n        process.exit(0);";
const replacement = "        client.stop();\n        setTimeout(() => process.exit(0), 300);";

const source = fs.readFileSync(indexPath, "utf8");
const occurrences = source.split(target).length - 1;
if (occurrences !== 2) {
  throw new Error(`outray graceful-shutdown patch expected 2 occurrences of the target in ${indexPath}, found ${occurrences} — CLI source changed, update the patch`);
}
fs.writeFileSync(indexPath, source.split(target).join(replacement));
