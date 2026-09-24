#!/usr/bin/env bash
set -euo pipefail

: "${YELLOW_CARD_PUBLIC_KEY:?}" "${YELLOW_CARD_SECRET_KEY:?}"
BASE="${YELLOW_CARD_BASE_URL:-https://sandbox.api.yellowcard.io/business}"
BASE="${BASE%/}"

# The signature covers the full URL path, /business included, so the base URL's
# own path prefix has to be recovered and prepended to each endpoint.
hostpath="${BASE#*://}"
if [ "$hostpath" = "${hostpath#*/}" ]; then
  BASE_PATH=""
else
  BASE_PATH="/${hostpath#*/}"
fi

usage="usage: $0 <command>

sends:
  tx <payment_id>            GET /send/{id}
  seq <sequence_id>          GET /send/sequence-id/{id}
  list [per_page] [start_at] GET /sends/
  all [per_page]             page through /sends/ into one JSON array
  fail-liquidity <id>        POST /send/{id}/fail-pending-liquidity

receives:
  rx <collection_id>         GET /receive/{id}
  rx-seq <sequence_id>       GET /receive/sequence-id/{id}
  rx-list [per_page]         GET /receives/

reference:
  account | channels [country] | networks [country] | rates [currency]

escape hatches:
  get <path> [query]
  post <path> [body_file|-]"

# YcHmacV1 = base64(HMAC-SHA256(secret, timestamp + path + METHOD [+ base64(sha256(body))])).
# The query string is excluded from the signed path; the timestamp must be
# within five minutes of YellowCard's clock.
sign() {
  local ts="$1" method="$2" endpoint="$3" body="${4-}" msg
  msg="${ts}${BASE_PATH}${endpoint}${method}"
  if [ -n "$body" ]; then
    msg+=$(printf '%s' "$body" | openssl dgst -sha256 -binary | openssl base64 -A)
  fi
  printf '%s' "$msg" |
    openssl dgst -sha256 -hmac "$YELLOW_CARD_SECRET_KEY" -binary | openssl base64 -A
}

# raw emits the response body alone, for piping; call adds -i for humans.
raw() {
  local method="$1" endpoint="$2" query="${3-}" body="${4-}"
  local ts sig url
  ts=$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)
  sig=$(sign "$ts" "$method" "$endpoint" "$body")
  url="${BASE}${endpoint}"
  [ -n "$query" ] && url="${url}?${query}"

  set -- -sS ${SHOW_HEADERS:+-i} -X "$method" "$url" \
    -H "X-YC-Timestamp: $ts" \
    -H "Authorization: YcHmacV1 ${YELLOW_CARD_PUBLIC_KEY}:${sig}" \
    -H "Accept: application/json"
  if [ -n "$body" ]; then
    set -- "$@" -H "Content-Type: application/json" --data-binary "$body"
  fi
  curl "$@"
}

call() {
  SHOW_HEADERS=1 raw "$@"
  echo
}

# /sends/ pages by integer offset and returns {"payments": [...]}; a short page
# is the last one.
page_all() {
  local per="${1:-100}" at=0 page n tmp
  command -v jq >/dev/null || { echo "all requires jq" >&2; exit 1; }
  tmp=$(mktemp)
  trap 'rm -f "$tmp"' RETURN
  while :; do
    page=$(raw GET "/sends/" "perPage=${per}&startAt=${at}")
    n=$(printf '%s' "$page" | jq '.payments | length')
    [ "$n" -eq 0 ] && break
    printf '%s' "$page" | jq -c '.payments' >>"$tmp"
    at=$((at + n))
    echo "fetched $at" >&2
    [ "$n" -lt "$per" ] && break
  done
  jq -s 'add // [] | unique_by(.id)' "$tmp"
}

CMD="${1:?$usage}"

case "$CMD" in
tx)             call GET "/send/${2:?$usage}" ;;
seq)            call GET "/send/sequence-id/${2:?$usage}" ;;
list)           call GET "/sends/" "perPage=${2:-50}&startAt=${3:-0}" ;;
all)            page_all "${2:-100}" ;;
fail-liquidity) call POST "/send/${2:?$usage}/fail-pending-liquidity" ;;
rx)             call GET "/receive/${2:?$usage}" ;;
rx-seq)         call GET "/receive/sequence-id/${2:?$usage}" ;;
rx-list)        call GET "/receives/" "perPage=${2:-50}" ;;
account)        call GET "/account" ;;
channels)       call GET "/channels" "${2:+country=$2}" ;;
networks)       call GET "/networks" "${2:+country=$2}" ;;
rates)          call GET "/rates" "${2:+currency=$2}" ;;
get)            call GET "${2:?$usage}" "${3-}" ;;
post)
  body=""
  if [ $# -ge 3 ]; then
    if [ "$3" = "-" ]; then body=$(cat); else body=$(cat "$3"); fi
  fi
  call POST "${2:?$usage}" "" "$body"
  ;;
*)
  echo "$usage" >&2
  exit 2
  ;;
esac
