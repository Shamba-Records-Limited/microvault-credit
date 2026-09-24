#!/usr/bin/env bash
set -euo pipefail

: "${MG_BASE_URL:?}" "${MG_AUTH_PUBLIC_KEY:?}" "${MG_AUTH_SECRET_KEY:?}"
BASE="${MG_BASE_URL%/}"
SIGNER_DIR="${HOME}/.cache/mg-sep10"
STELLAR_BASE="$SIGNER_DIR/node_modules/@stellar/stellar-base"

if [ ! -d "$STELLAR_BASE" ]; then
  mkdir -p "$SIGNER_DIR"
  npm i --silent --prefix "$SIGNER_DIR" @stellar/stellar-base >/dev/null
fi

challenge=$(curl -fsS "$BASE/auth?account=$MG_AUTH_PUBLIC_KEY")

sign_js='
const sdk = require("@stellar/stellar-base");
const c = JSON.parse(process.env.CHALLENGE);
const tx = sdk.TransactionBuilder.fromXDR(c.transaction, c.network_passphrase);
tx.sign(sdk.Keypair.fromSecret(process.env.MG_AUTH_SECRET_KEY));
process.stdout.write(tx.toXDR());
'
signed=$(NODE_PATH="$SIGNER_DIR/node_modules" CHALLENGE="$challenge" \
  node -e "$sign_js")

token_js='JSON.parse(require("fs").readFileSync(0)).token'
TOKEN=$(curl -fsS -X POST "$BASE/auth" \
  -H "Content-Type: application/json" \
  -d "{\"transaction\":\"$signed\"}" | node -pe "$token_js")

usage="usage: $0 <stellar_tx_hash> | list [limit] | all"
ARG="${1:?$usage}"

all_js='
const base = process.env.BASE, auth = "Bearer " + process.env.TOKEN;
(async () => {
  const all = [];
  let paging = "";
  for (;;) {
    let url = base + "/sep24/transactions?asset_code=USDC&limit=100";
    if (paging) url += "&paging_id=" + paging;
    const res = await fetch(url, { headers: { Authorization: auth } });
    const body = await res.text();
    if (!res.ok || !body.startsWith("{")) {
      throw new Error("page failed " + res.status + ": " + body.slice(0, 200));
    }
    const page = JSON.parse(body).transactions || [];
    const fresh = page.filter((t) => !all.some((a) => a.id === t.id));
    if (!fresh.length) break;
    all.push(...fresh);
    paging = page[page.length - 1].id;
    console.error("fetched " + all.length);
  }
  process.stdout.write(JSON.stringify(all, null, 2) + "\n");
})().catch((e) => { console.error(e.message); process.exit(1); });
'

if [ "$ARG" = "all" ]; then
  BASE="$BASE" TOKEN="$TOKEN" node -e "$all_js"
  exit
fi

if [ "$ARG" = "list" ]; then
  QUERY="asset_code=USDC&limit=${2:-50}"
  URL="$BASE/sep24/transactions?$QUERY"
else
  URL="$BASE/sep24/transaction?stellar_transaction_id=$ARG"
fi

curl -sS -i "$URL" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json"
echo
