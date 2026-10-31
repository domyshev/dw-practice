# Local development and verification

Use `npm ci` to install frontend dependencies. Use `npm run dev:mock` for isolated UI development and browser checks without email credentials or real journal data.

- Mock web: `http://127.0.0.1:5174/`; mock API: `http://127.0.0.1:8081/api`.
- Seeded account: `demo@dwpractice.test`; enter any six-digit code (for example `123456`) or use any nonempty password.
- Use `second@dwpractice.test` for account isolation and `empty@dwpractice.test` for empty states.
- Use `npm run mock:reset` to restore fixtures. It invalidates mock sessions, so reload and sign in again.
- Use `npm run mock:smoke` against the running mock API for a full HTTP login/CRUD/audit check.
- Stop the combined runner with Ctrl+C; it owns and stops its Go and Vite children.
- The regular development app on 5173/8080 is independent. Do not replace, reset or reuse its database for mock checks.
- Never enable mock authentication in production. Mock startup refuses `APP_ENV=production`.
- Run the checks appropriate to the change: `npm test`, `go test -race -count=1 ./...`, `go vet ./...`, `npm run build`.

Fixture definitions, custom ports and the mock-only reset endpoint are documented in `docs/MOCK_SERVER.md`.
