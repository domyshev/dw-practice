# HTTP API

All paths begin with `/api`. Authenticated requests use the `karma_session` cookie. JSON responses include `Cache-Control: no-store`. Mutations use `Content-Type: application/json` and, in production, an `Origin` equal to `APP_ORIGIN`. Errors have the shape `{"error":"message"}`. There is no public cross-origin API in this first version.

| Method | Path | Result |
|---|---|---|
| GET | /health | Database readiness and email delivery mode |
| POST | /auth/request | `{ "email": "person@example.com" }`; returns 202 |
| POST | /auth/verify | `{ "email": "person@example.com", "code": "123456" }`; returns user and session cookie |
| POST | /auth/logout | Revokes current session |
| GET | /me | Current `{id,email}` |
| GET | /entries | Current user's active entries, newest first |
| POST | /entries | Creates entry and audit event in one transaction; returns 201 and `{entry}` |
| PUT | /entries/{id} | Replaces editable fields if `version` matches; returns `{entry}` |
| DELETE | /entries/{id}?version=N | Soft-deletes if version matches; returns `{entry:null}` |
| GET | /audit | Current user's complete before/after history, newest first |

Entry create/update input:

```json
{
  "practice": "prostrations",
  "large": 2,
  "small": 1,
  "repetitions": 4,
  "initial": false,
  "occurredAt": "2026-01-20T20:15:00+01:00",
  "timeZone": "Europe/Madrid",
  "note": "Evening practice",
  "version": 0
}
```

Practice IDs: `prostrations`, `diamond-mind`, `mandala`, `guru-yoga`. Counts must be non-negative integers; at least one repetition is required. Large/small counts are limited to 1,000,000 each and individual repetitions to 100,000,000. Notes are limited to 2,000 UTF-8 bytes. Dates must be between 1900 and the present (five-minute clock skew permitted). The supplied UTC offset must match the named IANA zone on that date.

The returned entry also contains `id`, server-derived `userId`, `total`, `offsetMinutes`, `createdAt`, `updatedAt`, and `version`. Never depend on a supplied ID, owner or total: the server controls them. For updates send the returned version and all editable fields. The original `initial` classification is retained. Foreign/deleted IDs return 404; stale versions return 409. Invalid entries return 422; unauthenticated requests return 401; throttled code requests return 429.

Audit events contain `{id,entryId,action,at,before,after}`. `action` is `created`, `updated`, or `deleted`. `at` is a server UTC timestamp. Creation has `before:null`; deletion has `after:null`; snapshots preserve complete entry fields, including the owning user's ID and original time zone.

An iPhone client can use this same service over HTTPS with a first-party cookie session. A future native client should send the configured first-party Origin or use a separately designed native-auth endpoint; do not weaken the web Origin policy to accommodate it. Offline synchronization will need a separate idempotent mutation and conflict-resolution contract.
