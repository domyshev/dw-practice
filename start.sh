#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")"
if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi
export APP_ORIGIN="${APP_ORIGIN:-http://127.0.0.1:8080}"
if [ ! -d node_modules ]; then npm ci; fi
npm run build
printf '\nOpen %s in your browser. Local login codes appear below.\n\n' "$APP_ORIGIN"
exec go run ./backend
