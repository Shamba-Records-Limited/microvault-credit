#!/bin/sh
set -e

if [ -z "$OUTRAY_TOKEN" ]; then
	echo "OUTRAY_TOKEN is required" >&2
	exit 1
fi

# outray exits "Not logged in" when ~/.outray/config.json is absent, and that
# check runs before --key is parsed.
mkdir -p "$HOME/.outray"
printf '{"authType":"user","userToken":"%s"}' "$OUTRAY_TOKEN" > "$HOME/.outray/config.json"
chmod 600 "$HOME/.outray/config.json"

exec outray "$@" --key "$OUTRAY_TOKEN"
