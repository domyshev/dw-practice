package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The integration suite refuses a production-looking database name. It runs
// against a dedicated database in CI and before the production cutover.
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || len(parsed.Path) < 6 || parsed.Path[len(parsed.Path)-5:] != "_test" {
		t.Fatal("TEST_DATABASE_URL must name a dedicated *_test database")
	}
	db, err := openPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", dsn)
	path := filepath.Join(t.TempDir(), "source.db")
	sqlite, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	secret := token()
	codes := map[string]string{}
	source := &app{db: sqlite, secret: secret, origin: "http://localhost", emailMode: "log", send: func(_ context.Context, email, code string) error { codes[email] = code; return nil }}
	h := source.handler()
	cookie := login(t, h, codes, "owner@example.com")
	first := create(t, h, cookie, validEntry())
	second := create(t, h, cookie, validEntry())
	if _, err = sqlite.Exec(`INSERT INTO create_requests(user_id,request_key,request_hash,entry_body) VALUES(?,?,?,?)`, second.UserID, "before-migration", digest("before-migration"), serialized(&second)); err != nil {
		t.Fatal(err)
	}
	second.Note = "Updated before migration"
	if w := call(h, "PUT", "/api/entries/"+second.ID, second, cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(h, "DELETE", "/api/entries/"+first.ID+"?version=1", nil, cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = migratePostgres(path); err != nil {
		t.Fatal(err)
	}
	if err = migratePostgres(path); err == nil {
		t.Fatal("accepted a duplicate import")
	}
	read, err := sqlite.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Rollback()
	defer sqlite.Close()
	dest, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range migrationTables {
		a, n, e := tableFingerprint(read, table)
		if e != nil {
			t.Fatal(e)
		}
		b, m, e := tableFingerprint(dest, table)
		if e != nil || a != b || n != m {
			t.Fatalf("mismatch in %s: %v", table.name, e)
		}
	}
	dest.Rollback()
	makeApp := func() *app {
		return &app{db: db, postgres: true, secret: secret, origin: "http://localhost", emailMode: "log", send: func(context.Context, string, string) error { return nil }}
	}
	a, b := makeApp(), makeApp()
	ha, hb := a.handler(), b.handler()
	if w := call(ha, "GET", "/api/me", nil, cookie); w.Code != 200 {
		t.Fatal("session not preserved", w.Code)
	}
	for _, path := range []string{"/api/entries", "/api/entries/page?practice=mandala&since=2020-01-01T00:00:00Z", "/api/summary?timeZone=Europe%2FMadrid", "/api/audit"} {
		if w := call(ha, "GET", path, nil, cookie); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	otherCodes := map[string]string{}
	b.send = func(_ context.Context, email, code string) error { otherCodes[email] = code; return nil }
	other := login(t, hb, otherCodes, "second@example.com")
	if w := call(hb, "PUT", "/api/entries/"+second.ID, second, other); w.Code != 404 {
		t.Fatal("cross-account mutation accepted", w.Code)
	}
	if w := call(hb, "GET", "/api/entries", nil, other); w.Body.String() != "[]\n" {
		t.Fatal("cross-account entries leaked", w.Body.String())
	}
	for _, statement := range []string{`UPDATE audit SET action='changed'`, `DELETE FROM audit`, `TRUNCATE audit`} {
		if _, err = db.Exec(statement); err == nil {
			t.Fatal("audit mutation accepted", statement)
		}
	}
	// Two independent app instances must return the same idempotent response.
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/api/entries", entryReader(validEntry()))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", "pg-concurrent-retry")
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			if i%2 == 0 {
				ha.ServeHTTP(w, r)
			} else {
				hb.ServeHTTP(w, r)
			}
			responses <- w
		}(i)
	}
	wg.Wait()
	close(responses)
	id := ""
	for w := range responses {
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v struct {
			Entry entry `json:"entry"`
		}
		json.Unmarshal(w.Body.Bytes(), &v)
		if id != "" && id != v.Entry.ID {
			t.Fatal("duplicate idempotent entries")
		}
		id = v.Entry.ID
	}
	// Cross-process stale writes must not overwrite one another.
	var current entry
	w := call(ha, "GET", "/api/entries", nil, cookie)
	var entries []entry
	json.Unmarshal(w.Body.Bytes(), &entries)
	for _, e := range entries {
		if e.ID == id {
			current = e
		}
	}
	statuses := make(chan int, 2)
	for _, handler := range []http.Handler{ha, hb} {
		wg.Add(1)
		go func(h http.Handler) {
			defer wg.Done()
			statuses <- call(h, "PUT", "/api/entries/"+id, current, cookie).Code
		}(handler)
	}
	wg.Wait()
	close(statuses)
	success, conflict := 0, 0
	for status := range statuses {
		if status == 200 {
			success++
		}
		if status == 409 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("stale writes: success=%d conflict=%d", success, conflict)
	}
	// OTP consumption and rate limits must also hold across app instances.
	codeID, code, err := a.prepareCode("concurrent@example.com", "198.51.100.1")
	if err != nil || codeID < 3 {
		t.Fatal("sequence not advanced after import", codeID, err)
	}
	statuses = make(chan int, 2)
	for _, handler := range []http.Handler{ha, hb} {
		wg.Add(1)
		go func(h http.Handler) {
			defer wg.Done()
			statuses <- call(h, "POST", "/api/auth/verify", map[string]string{"email": "concurrent@example.com", "code": code}, nil).Code
		}(handler)
	}
	wg.Wait()
	close(statuses)
	success, conflict = 0, 0
	for status := range statuses {
		if status == 200 {
			success++
		}
		if status == 401 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("OTP reuse: success=%d rejected=%d", success, conflict)
	}
	a.emailMode = "resend"
	b.emailMode = "resend"
	if _, _, err = a.prepareCode("limited@example.com", "198.51.100.2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = b.prepareCode("limited@example.com", "198.51.100.3"); err != errCodeRateLimited {
		t.Fatal("cross-instance resend limit failed", err)
	}
	// A failed audit insert must roll back both the entry and idempotency record.
	if _, err = db.Exec(`CREATE FUNCTION fail_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected audit failure'; END $$; CREATE TRIGGER fail_test_audit BEFORE INSERT ON audit FOR EACH STATEMENT EXECUTE FUNCTION fail_test_audit()`); err != nil {
		t.Fatal(err)
	}
	var before, after int
	db.QueryRow(`SELECT count(*) FROM entries`).Scan(&before)
	if w := call(ha, "POST", "/api/entries", validEntry(), cookie); w.Code != 500 {
		t.Fatal("audit failure accepted", w.Code)
	}
	db.QueryRow(`SELECT count(*) FROM entries`).Scan(&after)
	if before != after {
		t.Fatal("entry survived audit rollback")
	}
	if _, err = db.Exec(`DROP TRIGGER fail_test_audit ON audit; DROP FUNCTION fail_test_audit()`); err != nil {
		t.Fatal(err)
	}
	if w := call(hb, "GET", "/api/me", nil, cookie); w.Code != 200 {
		t.Fatal("session unavailable on second instance")
	}
	var imported sql.NullString
	if err = db.QueryRow(`SELECT imported_at FROM schema_version WHERE id=1`).Scan(&imported); err != nil || !imported.Valid {
		t.Fatal("missing import marker", err)
	}
}

func entryReader(e entry) *bytes.Reader { body, _ := json.Marshal(e); return bytes.NewReader(body) }
