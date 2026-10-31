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
	a.emailMode = "resend"
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

func TestSlowDeliveryDoesNotBlockVerificationOrJournalWrites(t *testing.T) {
	a, h, codes := fixture(t)
	cookie := login(t, h, codes, "writer@example.com")
	if w := call(h, "POST", "/api/auth/request", map[string]string{"email": "verify@example.com"}, nil); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	verifyCode := codes["verify@example.com"]
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	a.send = func(context.Context, string, string) error { close(started); <-release; return nil }
	delivery := make(chan int, 1)
	go func() {
		delivery <- call(h, "POST", "/api/auth/request", map[string]string{"email": "slow@example.com"}, nil).Code
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("delivery did not start")
	}
	writes, verification := make(chan int, 1), make(chan int, 1)
	go func() { writes <- call(h, "POST", "/api/entries", validEntry(), cookie).Code }()
	go func() {
		verification <- call(h, "POST", "/api/auth/verify", map[string]string{"email": "verify@example.com", "code": verifyCode}, nil).Code
	}()
	for name, result := range map[string]<-chan int{"write": writes, "verification": verification} {
		select {
		case status := <-result:
			if status != 200 && status != 201 {
				t.Fatalf("%s failed: %d", name, status)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s blocked by email delivery", name)
		}
	}
}

func TestFailedOlderDeliveryDoesNotInvalidateNewCode(t *testing.T) {
	a, h, _ := fixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var calls int
	var currentCode string
	var deliveryMu sync.Mutex
	a.send = func(_ context.Context, _ string, code string) error {
		deliveryMu.Lock()
		calls++
		first := calls == 1
		if !first {
			currentCode = code
		}
		deliveryMu.Unlock()
		if first {
			close(started)
			<-release
			return fmt.Errorf("old delivery failed")
		}
		return nil
	}
	oldRequest := make(chan int, 1)
	go func() {
		oldRequest <- call(h, "POST", "/api/auth/request", map[string]string{"email": "person@example.com"}, nil).Code
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("delivery did not start")
	}
	if _, err := a.db.Exec(`UPDATE codes SET created_at=?`, time.Now().Unix()-61); err != nil {
		close(release)
		t.Fatal(err)
	}
	newRequest := make(chan int, 1)
	go func() {
		newRequest <- call(h, "POST", "/api/auth/request", map[string]string{"email": "person@example.com"}, nil).Code
	}()
	select {
	case status := <-newRequest:
		if status != 202 {
			close(release)
			t.Fatal(status)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("new code request blocked")
	}
	close(release)
	select {
	case status := <-oldRequest:
		if status != 502 {
			t.Fatal(status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("old delivery did not finish")
	}
	deliveryMu.Lock()
	code := currentCode
	deliveryMu.Unlock()
	if w := call(h, "POST", "/api/auth/verify", map[string]string{"email": "person@example.com", "code": code}, nil); w.Code != 200 {
		t.Fatal("older failure invalidated the new code", w.Body.String())
	}
}

func callWithKey(h http.Handler, e entry, cookie *http.Cookie, key string) *httptest.ResponseRecorder {
	var b bytes.Buffer
	_ = json.NewEncoder(&b).Encode(e)
	r := httptest.NewRequest("POST", "/api/entries", &b)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestIdempotentCreateRetriesAndConflict(t *testing.T) {
	a, h, codes := fixture(t)
	c := login(t, h, codes, "one@example.com")
	e := validEntry()
	first := callWithKey(h, e, c, "operation-1")
	retry := callWithKey(h, e, c, "operation-1")
	if first.Code != 201 || retry.Code != 201 || first.Body.String() != retry.Body.String() {
		t.Fatal("retry must return the same entry", first.Body.String(), retry.Body.String())
	}
	e.Repetitions++
	if w := callWithKey(h, e, c, "operation-1"); w.Code != 409 {
		t.Fatal("reused key accepted different payload", w.Code)
	}
	for _, table := range []string{"entries", "audit", "create_requests"} {
		var count int
		if err := a.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s: %d, %v", table, count, err)
		}
	}
}

func TestIdempotencyKeysAreScopedToUser(t *testing.T) {
	_, h, codes := fixture(t)
	a := login(t, h, codes, "a@example.com")
	b := login(t, h, codes, "b@example.com")
	first := callWithKey(h, validEntry(), a, "shared-key")
	second := callWithKey(h, validEntry(), b, "shared-key")
	if first.Code != 201 || second.Code != 201 {
		t.Fatal(first.Code, second.Code)
	}
	var left, right struct {
		Entry entry `json:"entry"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &left)
	_ = json.Unmarshal(second.Body.Bytes(), &right)
	if left.Entry.ID == right.Entry.ID || left.Entry.UserID == right.Entry.UserID {
		t.Fatal("idempotency leaked across accounts")
	}
}

func TestConcurrentIdempotentCreate(t *testing.T) {
	a, h, codes := fixture(t)
	c := login(t, h, codes, "a@example.com")
	results := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- callWithKey(h, validEntry(), c, "same-operation") }()
	}
	wg.Wait()
	close(results)
	var body string
	for w := range results {
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		if body != "" && w.Body.String() != body {
			t.Fatal("concurrent requests created different entries")
		}
		body = w.Body.String()
	}
	var count int
	if err := a.db.QueryRow(`SELECT count(*) FROM audit`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestFailedAuditRollsBackIdempotencyKey(t *testing.T) {
	a, h, codes := fixture(t)
	c := login(t, h, codes, "a@example.com")
	if _, err := a.db.Exec(`CREATE TRIGGER fail_idempotent_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if w := callWithKey(h, validEntry(), c, "retry-after-failure"); w.Code != 500 {
		t.Fatal(w.Code)
	}
	var count int
	if err := a.db.QueryRow(`SELECT count(*) FROM create_requests`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed write retained key", count, err)
	}
	if _, err := a.db.Exec(`DROP TRIGGER fail_idempotent_audit`); err != nil {
		t.Fatal(err)
	}
	if w := callWithKey(h, validEntry(), c, "retry-after-failure"); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestLocalLogModeAllowsRepeatedCodeRequests(t *testing.T) {
	a, h, codes := fixture(t)
	a.emailMode = "log"
	for i := 0; i < 8; i++ {
		w := call(h, "POST", "/api/auth/request", map[string]string{"email": "local@example.com"}, nil)
		if w.Code != 202 {
			t.Fatalf("local request %d throttled: %d %s", i+1, w.Code, w.Body.String())
		}
	}
	w := call(h, "POST", "/api/auth/verify", map[string]string{"email": "local@example.com", "code": codes["local@example.com"]}, nil)
	if w.Code != 200 {
		t.Fatal("latest local code should authenticate", w.Code, w.Body.String())
	}
}
