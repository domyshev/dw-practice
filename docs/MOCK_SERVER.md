# Local mock server

The mock server runs the same Go API and validation as the normal backend, with isolated disposable SQLite data in memory. It does not read `.env`, use `DATABASE_PATH`, access a real journal, contact an email provider, or need credentials. All mock-only features require `--mock`; startup refuses `APP_ENV=production` and the API binds only to `127.0.0.1`.

## Start both servers

Requirements: Node.js 22.12+ and Go 1.24+.

```sh
npm ci
npm run dev:mock
```

Open **http://127.0.0.1:5174/**. The runner starts the mock API at **http://127.0.0.1:8081/api**, waits for readiness and then starts Vite with the mock proxy. Both ports are strict; occupied ports produce an error instead of silently selecting another port. Ctrl+C stops both processes and discards their in-memory data. The normal 5173/8080 development servers may keep running alongside them.

For separate terminals:

```sh
npm run mock:api
npm run mock:web
```

The mock UI is visibly labelled “Mock data”. Its login screen explains that the data is disposable and displays the fixed code. It uses a separate `dw_mock_session` cookie, so signing into the mock app does not replace the normal app's `karma_session` cookie. Any six-digit numeric code (for example `123456` or `999999`) always works in mock mode, even without a prior request or after previous sign-ins. The mock-only `/api/mock/login` endpoint also accepts any syntactically valid email and any nonempty password. The UI offers “Use email and any password”; passwords are never stored or logged. Both shortcuts are absent from the normal API.

## Accounts and fixtures

| Email | Code | Data |
|---|---|---|
| `demo@dwpractice.test` | `123456` | 14 active entries across four practices; four starting balances; activity over the last week; multiple IANA zones; 17 audit events including an edit and a deleted entry |
| `second@dwpractice.test` | `123456` | A separate account with two entries and isolated history |
| `empty@dwpractice.test` | `123456` | Created at first sign-in with no entries, for testing empty states |

Other valid emails create separate empty mock accounts. No email is ever sent. Use reserved `.test` addresses for fixtures.

Seed definitions are versioned in `backend/mockdata/seed.json` and embedded into the mock executable. Their IDs and counts are fixed, while dates are relative to server startup, keeping today's and weekly activity useful. `daysAgo`, `minutesAgo` and `timeZone` control dates; the default zone is `Europe/Madrid`. Included zones also cover `Asia/Kathmandu` and `America/New_York`. `editRepetitions` creates an update snapshot; `deleted` creates a soft deletion with a retained audit record. Restart the API after editing the seed file.

Supported operations use the ordinary [API contract](API.md): login/logout, user lookup, list/create/update/delete entries, version conflicts, idempotency keys, journal export through the frontend, and before/after audit history.

## Reset

With the mock server running:

```sh
npm run mock:reset
```

This checks that the target identifies itself as a mock, restores all fixture accounts/entries/audit events, and removes temporary accounts, sessions, codes and idempotency records. Reload open mock tabs and sign in again. The command never resets the normal database. Restarting the mock server also restores its original fixtures.

## Automated agent smoke check

With `npm run dev:mock` running in another terminal:

```sh
npm run mock:smoke
```

This checks mock health, signs into a unique empty test account with the fixed code, creates an entry, repeats the same creation idempotently, edits it, deletes it, checks its three audit events and signs out. It does not reset or edit the seeded demo accounts. The temporary smoke account and its history disappear on reset/restart.

Mock reset endpoint (not registered by the ordinary API):

```sh
curl -X POST http://127.0.0.1:8081/api/mock/reset \
  -H 'Content-Type: application/json' \
  -H 'Origin: http://127.0.0.1:5174' \
  -d '{}'
```

`GET /api/health` in mock mode adds `mock: true`, `loginCode: "123456"` and `demoEmail: "demo@dwpractice.test"`. These keys are absent from the normal API. Foreign browser origins cannot reset data, and the normal API returns 404 for the reset route.

## Alternative ports

```sh
MOCK_WEB_PORT=5274 MOCK_API_PORT=8181 npm run dev:mock
MOCK_WEB_PORT=5274 MOCK_API_PORT=8181 npm run mock:reset
MOCK_WEB_PORT=5274 MOCK_API_PORT=8181 npm run mock:smoke
```

When running the two servers separately on custom ports, also set `MOCK_APP_ORIGIN=http://127.0.0.1:5274` for the API. The combined runner sets it automatically. Mock-specific environment variables are independent of the normal `LISTEN_ADDR` and `APP_ORIGIN` settings.

## Tests

```sh
npm test
go test -race -count=1 ./...
go vet ./...
npm run build
```

Mock integration tests cover fixture contents, starting balances, fractional UTC offsets, edits/deletions in audit history, account isolation, empty accounts, always-valid fixed-code and arbitrary-password login, cookie isolation, repeated login requests, reset/session invalidation, retained audit protection, origin checks, missing mock routes/metadata on the normal API, production refusal and isolation from the configured database path.
