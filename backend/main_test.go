package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*app, http.Handler, map[string]string) {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	codes := map[string]string{}
	a := &app{db: db, secret: token(), origin: "http://localhost", emailMode: "log", send: func(_ context.Context, email, code string) error { codes[email] = code; return nil }}
	return a, a.handler(), codes
}
func call(h http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	var b bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&b).Encode(body)
	}
	r := httptest.NewRequest(method, path, &b)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, h http.Handler, codes map[string]string, email string) *http.Cookie {
	t.Helper()
	w := call(h, "POST", "/api/auth/request", map[string]string{"email": email}, nil)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(h, "POST", "/api/auth/verify", map[string]string{"email": email, "code": codes[email]}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}
func validEntry() entry {
	return entry{Practice: "mandala", Large: 2, Small: 1, Repetitions: 4, OccurredAt: "2026-01-20T20:15:00+01:00", TimeZone: "Europe/Madrid", Note: "Evening practice"}
}
func create(t *testing.T, h http.Handler, c *http.Cookie, e entry) entry {
	t.Helper()
	w := call(h, "POST", "/api/entries", e, c)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Entry entry `json:"entry"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Entry
}
func TestCountsAndTimeZones(t *testing.T) {
	e := validEntry()
	if err := validate(&e); err != nil {
		t.Fatal(err)
	}
	if e.Total != 247 || e.OffsetMinutes != 60 {
		t.Fatalf("Wrong count or offset: %+v", e)
	}
	e.OccurredAt = "2026-07-20T20:15:00+01:00"
	if validate(&e) == nil {
		t.Fatal("accepted wrong summer offset")
	}
	e.OccurredAt = "2026-07-20T20:15:00+02:00"
	if err := validate(&e); err != nil {
		t.Fatal(err)
	}
	e.OccurredAt = "2026-03-29T02:30:00+01:00"
	if validate(&e) == nil {
		t.Fatal("accepted nonexistent DST time")
	}
	e = validEntry()
	e.TimeZone = "Asia/Kathmandu"
	e.OccurredAt = "2026-01-20T20:15:00+05:45"
	if err := validate(&e); err != nil {
		t.Fatal(err)
	}
	if e.OffsetMinutes != 345 {
		t.Fatal(e.OffsetMinutes)
	}
}
func TestInvalidCounts(t *testing.T) {
	for _, adjust := range []func(*entry){func(e *entry) { e.Large = -1 }, func(e *entry) { e.Repetitions = 100000001 }, func(e *entry) { e.Large = 0; e.Small = 0; e.Repetitions = 0 }, func(e *entry) { e.Practice = "unknown" }, func(e *entry) { e.TimeZone = "Local" }, func(e *entry) { e.OccurredAt = time.Now().Add(24 * time.Hour).Format(time.RFC3339) }} {
		e := validEntry()
		adjust(&e)
		if validate(&e) == nil {
			t.Fatalf("accepted %+v", e)
		}
	}
}
func TestOTPAndLogout(t *testing.T) {
	_, h, codes := fixture(t)
	c := login(t, h, codes, "person@example.com")
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe cookie")
	}
	w := call(h, "POST", "/api/auth/verify", map[string]string{"email": "person@example.com", "code": codes["person@example.com"]}, nil)
	if w.Code != 401 {
		t.Fatal("code reused")
	}
	if call(h, "GET", "/api/me", nil, c).Code != 200 {
		t.Fatal("session not recognized")
	}
	if call(h, "POST", "/api/auth/logout", nil, c).Code != 200 {
		t.Fatal("logout failed")
	}
	if call(h, "GET", "/api/me", nil, c).Code != 401 {
		t.Fatal("session survived logout")
	}
}
func TestOTPAttemptsAndExpiry(t *testing.T) {
	a, h, codes := fixture(t)
	call(h, "POST", "/api/auth/request", map[string]string{"email": "a@example.com"}, nil)
	wrong := "000000"
	if codes["a@example.com"] == wrong {
		wrong = "000001"
	}
	for i := 0; i < 5; i++ {
		if call(h, "POST", "/api/auth/verify", map[string]string{"email": "a@example.com", "code": wrong}, nil).Code != 401 {
			t.Fatal("wrong code accepted")
		}
	}
	if call(h, "POST", "/api/auth/verify", map[string]string{"email": "a@example.com", "code": codes["a@example.com"]}, nil).Code != 401 {
		t.Fatal("lockout bypassed")
	}
	call(h, "POST", "/api/auth/request", map[string]string{"email": "b@example.com"}, nil)
	if _, err := a.db.Exec(`UPDATE codes SET expires_at=? WHERE email=?`, time.Now().Unix()-1, "b@example.com"); err != nil {
		t.Fatal(err)
	}
	if call(h, "POST", "/api/auth/verify", map[string]string{"email": "b@example.com", "code": codes["b@example.com"]}, nil).Code != 401 {
		t.Fatal("expired code accepted")
	}
}
func TestRateLimitAndFailedDelivery(t *testing.T) {
	a, h, _ := fixture(t)
	a.send = func(context.Context, string, string) error { return fmt.Errorf("service unavailable") }
	w := call(h, "POST", "/api/auth/request", map[string]string{"email": "a@example.com"}, nil)
	if w.Code != 502 {
		t.Fatal(w.Code)
	}
	var consumed int
	if err := a.db.QueryRow(`SELECT consumed FROM codes`).Scan(&consumed); err != nil || consumed != 1 {
		t.Fatal("failed-delivery code left active")
	}
	if call(h, "POST", "/api/auth/request", map[string]string{"email": "a@example.com"}, nil).Code != 429 {
		t.Fatal("resend cooldown bypassed")
	}
}
func TestCRUDIsolationAndAudit(t *testing.T) {
	a, h, codes := fixture(t)
	c1 := login(t, h, codes, "one@example.com")
	c2 := login(t, h, codes, "two@example.com")
	input := validEntry()
	input.Initial = true
	input.UserID = "spoofed-user"
	e := create(t, h, c1, input)
	if e.UserID == "spoofed-user" {
		t.Fatal("client controls owner")
	}
	if call(h, "GET", "/api/entries", nil, c2).Body.String() != "[]\n" {
		t.Fatal("data leaked")
	}
	if call(h, "GET", "/api/audit", nil, c2).Body.String() != "[]\n" {
		t.Fatal("audit leaked")
	}
	e.Repetitions = 10
	e.Initial = false
	w := call(h, "PUT", "/api/entries/"+e.ID, e, c2)
	if w.Code != 404 {
		t.Fatal("cross-user update allowed")
	}
	w = call(h, "PUT", "/api/entries/"+e.ID, e, c1)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Entry entry `json:"entry"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Entry.Total != 253 || !out.Entry.Initial || out.Entry.Version != 2 {
		t.Fatal("update invariants failed")
	}
	if call(h, "PUT", "/api/entries/"+e.ID, e, c1).Code != 409 {
		t.Fatal("stale update allowed")
	}
	if call(h, "DELETE", "/api/entries/"+e.ID+"?version=1", nil, c1).Code != 409 {
		t.Fatal("stale delete allowed")
	}
	if call(h, "DELETE", "/api/entries/"+e.ID+"?version=2", nil, c2).Code != 404 {
		t.Fatal("cross-user delete allowed")
	}
	if call(h, "DELETE", "/api/entries/"+e.ID+"?version=2", nil, c1).Code != 200 {
		t.Fatal("delete failed")
	}
	if call(h, "GET", "/api/entries", nil, c1).Body.String() != "[]\n" {
		t.Fatal("deleted entry visible")
	}
	var logs []audit
	w = call(h, "GET", "/api/audit", nil, c1)
	if err := json.Unmarshal(w.Body.Bytes(), &logs); err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 || logs[0].Action != "deleted" || logs[0].Before.Total != 253 || logs[1].Before.Total != 247 || logs[1].After.Total != 253 || logs[2].Before != nil {
		t.Fatalf("wrong audit: %+v", logs)
	}
	if _, err := a.db.Exec(`DELETE FROM audit`); err == nil {
		t.Fatal("audit deletion allowed")
	}
	if _, err := a.db.Exec(`UPDATE audit SET action='tampered'`); err == nil {
		t.Fatal("audit update allowed")
	}
}
func TestAuditFailureRollsBackEntry(t *testing.T) {
	a, h, codes := fixture(t)
	c := login(t, h, codes, "a@example.com")
	_, err := a.db.Exec(`CREATE TRIGGER fail_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if call(h, "POST", "/api/entries", validEntry(), c).Code != 500 {
		t.Fatal("expected failure")
	}
	if call(h, "GET", "/api/entries", nil, c).Body.String() != "[]\n" {
		t.Fatal("entry committed without audit")
	}
}
func TestOriginAndAuthentication(t *testing.T) {
	_, h, _ := fixture(t)
	if call(h, "GET", "/api/entries", nil, nil).Code != 401 {
		t.Fatal("authentication bypass")
	}
	r := httptest.NewRequest("POST", "/api/auth/request", bytes.NewBufferString(`{"email":"a@example.com"}`))
	r.Header.Set("Origin", "https://attacker.example")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign origin accepted")
	}
}
func TestConcurrentCodeUse(t *testing.T) {
	_, h, codes := fixture(t)
	call(h, "POST", "/api/auth/request", map[string]string{"email": "a@example.com"}, nil)
	code := codes["a@example.com"]
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- call(h, "POST", "/api/auth/verify", map[string]string{"email": "a@example.com", "code": code}, nil).Code
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for status := range results {
		if status == 200 {
			success++
		} else if status != 401 {
			t.Fatal(status)
		}
	}
	if success != 1 {
		t.Fatal("code consumed more than once")
	}
}
func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, secret: token(), origin: "http://localhost"}
	codes := map[string]string{}
	a.send = func(_ context.Context, email, code string) error { codes[email] = code; return nil }
	h := a.handler()
	c := login(t, h, codes, "a@example.com")
	e := create(t, h, c, validEntry())
	db.Close()
	db, err = openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a.db = db
	w := call(h, "GET", "/api/entries", nil, c)
	var entries []entry
	if err = json.Unmarshal(w.Body.Bytes(), &entries); err != nil || len(entries) != 1 || entries[0].ID != e.ID {
		t.Fatal("records did not survive restart")
	}
}
