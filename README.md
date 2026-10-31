# DW — practice journal

A working first web version of a personal journal for the four foundational practices in Karma Kagyu Diamond Way Buddhism, in the tradition of Lama Ole Nydahl. This is an independent tracker, not an official application or a source of practice instructions.

## Run locally

Requirements: Node.js 22.12+ and Go 1.24+. From this directory:

```sh
./start.sh
```

Open http://127.0.0.1:8080. Enter an email address, then use the six-digit code printed in the Go terminal. This is explicitly a local development mode: no email is sent. The actual account, session, records and audit events are persisted by the Go backend in `data/karma.db`; no journal data is stored in browser localStorage. Restarting preserves the journal. No account or example records are seeded.

To send real email, copy `.env.example` to `.env`, set `EMAIL_MODE=resend`, `RESEND_API_KEY` and `EMAIL_FROM` using a verified sender. `start.sh` loads `.env`; direct `go run` does not. Do not commit `.env`.

For frontend development, run the Go API with `APP_ORIGIN=http://127.0.0.1:5173 go run ./backend`, and `npm run dev` in another terminal. Vite proxies `/api` to port 8080. Use exactly the configured origin (127.0.0.1 and localhost are different origins).

## Mock development for developers and agents

```sh
npm ci
npm run dev:mock
```

Open http://127.0.0.1:5174/, sign in with `demo@dwpractice.test` and code `123456`. This runs the shared Go API on port 8081 with versioned fixtures and a disposable in-memory database; no email credentials or real data are used. A second account, empty-account flow, all four starting balances, recent activity, time-zone examples and complete edit/delete history are included.

Use `npm run mock:reset` to restore fixtures and `npm run mock:smoke` to verify HTTP login/CRUD/audit while the server is running. Mock sessions use a separate cookie, so the normal development app can run alongside the mock. See [the mock guide](docs/MOCK_SERVER.md) and [agent instructions](AGENTS.md).

## Included

- React 19, TypeScript, Material UI, Redux Toolkit, Go HTTP API and SQLite with WAL.
- Email sign-in and automatic registration through the same code flow; Resend delivery follows the approach in the driving-theory application referenced by the user.
- Prostrations, Diamond Mind, Mandala and Guru Yoga. Large mala = 108, small mala = 27. Both mala counts and individual repetitions may be combined in the same entry. The server calculates the total.
- Create, edit and soft-delete entries. Version checks prevent overwriting another session's changes.
- Explicit starting balances, included in lifetime/practice totals and excluded from daily/weekly activity. Their type cannot be silently changed while editing.
- Full original local datetime, IANA time zone and UTC offset on each entry. The backend validates the offset against that zone at that instant. It derives the owner from the session, never the request body.
- Unified practice history covering additions, edits and deletions, practice filters, weekly activity, and TSV/CSV/XLSX export. XLSX includes active entries and complete before/after history in separate worksheets.
- Transactional audit events containing before/after snapshots, actor association, entry ID and server UTC timestamp. SQLite triggers reject audit updates/deletions. Deleted entries remain in the database and audit log. This protects the application history; it is not cryptographic tamper evidence against a database administrator.
- Responsive layouts for phone, tablet and desktop; accessible form labels and reduced-motion support.

The history displays each entry's original time and zone. Daily/weekly aggregates use the current device zone and recalculate when the journal renders; saved entry timestamps do not change during travel. Starting balances can be added more than once, for example to import separate historical counts; each is clearly marked. There are no imposed completion targets.

## Authentication and deployment

Codes expire after 10 minutes, are stored as HMAC digests, are consumed atomically, and allow at most 5 attempts. Resending invalidates earlier codes. With email delivery enabled, requests are limited to 5 per email and 20 per socket IP per hour, with a 60-second resend cooldown. Local log delivery skips these limits so iterative development does not lock out the developer; this mode can bind only to loopback and is refused in production. Failed deliveries invalidate their code. A random 30-day session token is stored only as a digest in SQLite and sent in an HttpOnly, SameSite=Lax cookie. Production cookies are Secure. No permissive CORS is enabled; production mutations require the exact configured Origin and JSON bodies.

Production requires `APP_ENV=production`, `EMAIL_MODE=resend`, `SESSION_SECRET` of at least 32 random characters, `RESEND_API_KEY`, `EMAIL_FROM`, and an HTTPS `APP_ORIGIN`. Production refuses to start with development settings. Log delivery can bind only to loopback. The server serves the built React app from `dist/`.

A Dockerfile is included (not executed in this environment). Build with `docker build -t karma-practice .`. Supply the production variables, publish port 8080 behind an HTTPS reverse proxy, and mount persistent storage at `/app/data`. Back up SQLite using its backup API or `sqlite3 data/karma.db ".backup backup.db"`; do not copy a live main DB without accounting for WAL. Database directory permissions should remain restricted.

This first version is intended for one Go service instance with a persistent SQLite volume. Before a public cloud launch, configure a verified email domain, HTTPS, backups/restore, monitoring, retention/account deletion, and privacy terms. Requests behind a reverse proxy currently share the proxy socket IP for the IP rate limit; forwarded headers are deliberately not trusted. Configure trusted-proxy handling and a shared rate limiter before scaling. Larger-scale storage can migrate to PostgreSQL without changing the web/iPhone API. There is no offline sync, mobile app, or public deployment in this version.

## Verification

```sh
npm ci
npm run build
npm test
go test -race -count=1 ./...
go vet ./...
```

Frontend regression tests (`npm test`, Node.js 22.12+ or 24) simulate delayed journal responses, account switches, obsolete 401 errors, request cancellation, out-of-order loads, and successful saves followed by failed refreshes. Successful mutations update local entries and close the editor before refreshing. A separate refresh warning offers Retry load without repeating the write. Create requests use a per-user idempotency key retained for retries of the same unchanged form.

Backend integration tests use isolated temporary SQLite databases and cover arithmetic, IANA offsets including DST and fractional-hour zones, invalid inputs, login/logout, OTP reuse/expiry/lockout, resend limits and failed delivery, concurrent OTP use, record ownership, stale writes, full audit snapshots, audit immutability, rollback when audit writing fails, origin checks persistence across restart, nonblocking slow email delivery, failure of an older delivery after a newer code was issued, and idempotent create retry/isolation/concurrency/rollback.

Browser verification: request and verify a local sign-in code, open the journal, save 2 large malas + 1 small mala + 4 repetitions, confirm 247 repetitions and the original device zone in history, inspect the created audit event, add a 1,000-repetition starting balance and confirm lifetime total 1,247 while today/weekly totals remain 247. Browser QA uses a disposable account in a separate preview database outside the deliverable. Real Resend delivery and Docker deployment require external configuration and were not tested.

See `docs/API.md` for the HTTP contract. The iPhone roadmap is recorded separately in Russian as requested.

## Brand assets

The Diamond Way firewheel and favicon are original assets from the [official Diamond Way Buddhism website](https://www.diamondway-buddhism.org/). Sources: `https://www.diamondway-buddhism.org/images/DWB-firewheel-logo.jpg` and `https://www.diamondway-buddhism.org/wp-content/themes/dwb_theme/assets/img/dw-favicon-192x192.png`. The application wordmark is DW.

## Mobile interface and export

The interface starts with phone layouts. Recent entries are wrapping cards rather than a horizontally scrolling table. The header shows a clickable lowercase `dw` home link, the signed-in account and package version, followed by a menu. Its first control selects Light, System or Dark appearance; Practice, History and Sign out follow below. Theme preference is the only UI setting stored in localStorage. System mode follows operating-system changes.

History is the audit stream, including creation, edits, deletion and starting balances; select an event to see its full before/after snapshots. The export icon offers TSV, CSV and XLSX. Delimited files include all history operations and snapshots, UTF-8 BOM and safe quoting of user text. Formula-like text is escaped for spreadsheet applications. XLSX has Practice and Practice history worksheets with numeric counts and original local timestamps/zones. Its spreadsheet library loads only when XLSX is requested.

Browser layout checks used 430×932 (iPhone 15 Pro Max CSS viewport), 390×844 and 320×740 viewports. These are browser viewport checks, not physical-device certification. Export regression tests cover quoting, user text, deleted records, before/after data, workbook readback and empty journals.
