package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	_ "modernc.org/sqlite"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

type user struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
type entry struct {
	ID            string `json:"id"`
	UserID        string `json:"userId"`
	Practice      string `json:"practice"`
	Large         int    `json:"large"`
	Small         int    `json:"small"`
	Repetitions   int    `json:"repetitions"`
	Total         int    `json:"total"`
	Initial       bool   `json:"initial"`
	OccurredAt    string `json:"occurredAt"`
	TimeZone      string `json:"timeZone"`
	OffsetMinutes int    `json:"offsetMinutes"`
	Note          string `json:"note"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	Version       int    `json:"version"`
}
type audit struct {
	ID      string `json:"id"`
	EntryID string `json:"entryId"`
	Action  string `json:"action"`
	At      string `json:"at"`
	Before  *entry `json:"before"`
	After   *entry `json:"after"`
}
type app struct {
	db         *sql.DB
	secret     string
	origin     string
	production bool
	emailMode  string
	mock       bool
	send       func(context.Context, string, string) error
	mu         sync.Mutex
}
type key int

const userKey key = 1

var practices = map[string]bool{"prostrations": true, "diamond-mind": true, "mandala": true, "guru-yoga": true}

func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (a *app) codeHash(email, code string) string {
	h := hmac.New(sha256.New, []byte(a.secret))
	h.Write([]byte(email + ":" + code))
	return hex.EncodeToString(h.Sum(nil))
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, s string) {
	respond(w, status, map[string]string{"error": s})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "Invalid request body")
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
func internal(w http.ResponseWriter, err error) {
	log.Printf("request failed: %v", err)
	fail(w, 500, "Unable to complete the request")
}
func normalize(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	m, e := mail.ParseAddress(s)
	if e != nil || m.Address != s || len(s) > 254 {
		return "", errors.New("invalid email")
	}
	return s, nil
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,email TEXT UNIQUE NOT NULL,created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS codes(id INTEGER PRIMARY KEY,email TEXT NOT NULL,hash TEXT NOT NULL,ip TEXT NOT NULL,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,consumed INTEGER NOT NULL DEFAULT 0);
 CREATE INDEX IF NOT EXISTS codes_email ON codes(email,created_at);
 CREATE INDEX IF NOT EXISTS codes_ip ON codes(ip,created_at);
 CREATE TABLE IF NOT EXISTS sessions(hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS entries(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),body TEXT NOT NULL,occurred_at TEXT NOT NULL,deleted INTEGER NOT NULL DEFAULT 0);
 CREATE INDEX IF NOT EXISTS entries_user ON entries(user_id,deleted);
 CREATE INDEX IF NOT EXISTS entries_page ON entries(user_id,deleted,occurred_at DESC,id DESC);
 CREATE TABLE IF NOT EXISTS create_requests(user_id TEXT NOT NULL REFERENCES users(id),request_key TEXT NOT NULL,request_hash TEXT NOT NULL,entry_body TEXT NOT NULL,PRIMARY KEY(user_id,request_key));
 CREATE TABLE IF NOT EXISTS audit(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),entry_id TEXT NOT NULL,action TEXT NOT NULL,at TEXT NOT NULL,before_body TEXT,after_body TEXT);
 CREATE INDEX IF NOT EXISTS audit_user ON audit(user_id,at);
 CREATE TRIGGER IF NOT EXISTS audit_no_update BEFORE UPDATE ON audit BEGIN SELECT RAISE(ABORT,'Audit records are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS audit_no_delete BEFORE DELETE ON audit BEGIN SELECT RAISE(ABORT,'Audit records are immutable'); END;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/request", a.requestCode)
	mux.HandleFunc("POST /api/auth/verify", a.verifyCode)
	mux.Handle("GET /api/me", a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { respond(w, 200, r.Context().Value(userKey)) })))
	mux.Handle("POST /api/auth/logout", a.auth(http.HandlerFunc(a.logout)))
	mux.Handle("GET /api/entries/page", a.auth(http.HandlerFunc(a.pageEntries)))
	mux.Handle("GET /api/summary", a.auth(http.HandlerFunc(a.practiceSummary)))
	mux.Handle("GET /api/entries", a.auth(http.HandlerFunc(a.listEntries)))
	mux.Handle("POST /api/entries", a.auth(http.HandlerFunc(a.mutate)))
	mux.Handle("PUT /api/entries/{id}", a.auth(http.HandlerFunc(a.mutate)))
	mux.Handle("DELETE /api/entries/{id}", a.auth(http.HandlerFunc(a.mutate)))
	mux.Handle("GET /api/audit", a.auth(http.HandlerFunc(a.listAudit)))
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.db.Ping(); err != nil {
			internal(w, err)
			return
		}
		payload := map[string]any{"status": "ok", "emailMode": a.emailMode}
		if a.mock {
			payload["mock"] = true
			payload["loginCode"] = mockLoginCode
			payload["demoEmail"] = mockDemoEmail
		}
		respond(w, 200, payload)
	})
	if a.mock && !a.production {
		mux.HandleFunc("POST /api/mock/reset", a.resetMock)
		mux.HandleFunc("POST /api/mock/login", a.mockPasswordLogin)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "Endpoint not found") })
	files := http.FileServer(http.Dir("dist"))
	mux.Handle("/", files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if (origin != "" && origin != a.origin) || (a.production && origin == "") {
				fail(w, 403, "Untrusted request origin")
				return
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				fail(w, 403, "Cross-site requests are not allowed")
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				fail(w, 415, "Use application/json")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *app) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(a.sessionCookieName())
		if err != nil {
			fail(w, 401, "Please sign in")
			return
		}
		var u user
		err = a.db.QueryRow(`SELECT u.id,u.email FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=? AND s.expires_at>?`, digest(c.Value), time.Now().Unix()).Scan(&u.ID, &u.Email)
		if errors.Is(err, sql.ErrNoRows) {
			fail(w, 401, "Please sign in")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}
func (a *app) requestCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	email, err := normalize(in.Email)
	if err != nil {
		fail(w, 422, "Enter a valid email address")
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	id, code, err := a.prepareCode(email, ip)
	if errors.Is(err, errCodeRateLimited) {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "Too many requests. Please try again later.")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = a.send(r.Context(), email, code); err != nil {
		_, _ = a.db.Exec(`UPDATE codes SET consumed=1 WHERE id=?`, id)
		log.Printf("email delivery failed: %v", err)
		fail(w, 502, "Email delivery failed. Please try again later.")
		return
	}
	respond(w, 202, map[string]string{"message": "Your sign-in code is on its way"})
}

var errCodeRateLimited = errors.New("code request rate limited")

// Only the rate-limit check and code insertion need the shared lock.
// External delivery must not block other users' verification or journal writes.
func (a *app) prepareCode(email, ip string) (int64, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t := time.Now().Unix()
	var emailCount, ipCount int
	var latest int64
	if err := a.db.QueryRow(`SELECT count(*),coalesce(max(created_at),0) FROM codes WHERE email=? AND created_at>?`, email, t-3600).Scan(&emailCount, &latest); err != nil {
		return 0, "", err
	}
	if err := a.db.QueryRow(`SELECT count(*) FROM codes WHERE ip=? AND created_at>?`, ip, t-3600).Scan(&ipCount); err != nil {
		return 0, "", err
	}
	if (a.production || a.emailMode != "log") && (emailCount >= 5 || ipCount >= 20 || t-latest < 60) {
		return 0, "", errCodeRateLimited
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return 0, "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	if a.mock && !a.production {
		code = mockLoginCode
	}
	tx, err := a.db.Begin()
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE codes SET consumed=1 WHERE email=?`, email); err != nil {
		return 0, "", err
	}
	res, err := tx.Exec(`INSERT INTO codes(email,hash,ip,created_at,expires_at) VALUES(?,?,?,?,?)`, email, a.codeHash(email, code), ip, t, t+600)
	if err != nil {
		return 0, "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	if err = tx.Commit(); err != nil {
		return 0, "", err
	}
	return id, code, nil
}

func (a *app) verifyCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	email, err := normalize(in.Email)
	if err != nil || len(in.Code) != 6 {
		fail(w, 401, "Invalid or expired code")
		return
	}
	if a.mock && !a.production {
		for _, digit := range in.Code {
			if digit < '0' || digit > '9' {
				fail(w, 401, "Enter any six-digit mock code")
				return
			}
		}
		a.mockSignIn(w, r, email)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback()
	var id, expires int64
	var hash string
	var attempts int
	err = tx.QueryRow(`SELECT id,hash,expires_at,attempts FROM codes WHERE email=? AND consumed=0 ORDER BY id DESC LIMIT 1`, email).Scan(&id, &hash, &expires, &attempts)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		internal(w, err)
		return
	}
	if err != nil || expires <= time.Now().Unix() || attempts >= 5 || !hmac.Equal([]byte(hash), []byte(a.codeHash(email, in.Code))) {
		if id != 0 {
			if _, err = tx.Exec(`UPDATE codes SET attempts=attempts+1 WHERE id=?`, id); err != nil {
				internal(w, err)
				return
			}
			if err = tx.Commit(); err != nil {
				internal(w, err)
				return
			}
		}
		fail(w, 401, "Invalid or expired code")
		return
	}
	if _, err = tx.Exec(`INSERT INTO users(id,email,created_at) VALUES(?,?,?) ON CONFLICT(email) DO NOTHING`, token(), email, now()); err != nil {
		internal(w, err)
		return
	}
	var u user
	if err = tx.QueryRow(`SELECT id,email FROM users WHERE email=?`, email).Scan(&u.ID, &u.Email); err != nil {
		internal(w, err)
		return
	}
	session := token()
	if _, err = tx.Exec(`INSERT INTO sessions(hash,user_id,expires_at) VALUES(?,?,?)`, digest(session), u.ID, time.Now().Add(30*24*time.Hour).Unix()); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(`UPDATE codes SET consumed=1 WHERE id=?`, id); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.sessionCookieName(), Value: session, Path: "/", HttpOnly: true, Secure: a.production, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 86400})
	respond(w, 200, u)
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(a.sessionCookieName())
	if _, err := a.db.Exec(`DELETE FROM sessions WHERE hash=?`, digest(c.Value)); err != nil {
		internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.sessionCookieName(), Value: "", Path: "/", HttpOnly: true, Secure: a.production, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	respond(w, 200, map[string]bool{"ok": true})
}
func (a *app) listEntries(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userKey).(user)
	rows, err := a.db.Query(`SELECT body FROM entries WHERE user_id=? AND deleted=0 ORDER BY occurred_at DESC`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []entry{}
	for rows.Next() {
		var body string
		var e entry
		if err = rows.Scan(&body); err != nil {
			internal(w, err)
			return
		}
		if err = json.Unmarshal([]byte(body), &e); err != nil {
			internal(w, err)
			return
		}
		out = append(out, e)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, out)
}
func validate(e *entry) error {
	if !practices[e.Practice] {
		return errors.New("Choose one of the four practices")
	}
	if e.Large < 0 || e.Small < 0 || e.Repetitions < 0 || e.Large > 1000000 || e.Small > 1000000 || e.Repetitions > 100000000 {
		return errors.New("Counts must be non-negative whole numbers within the allowed range")
	}
	e.Total = e.Large*108 + e.Small*27 + e.Repetitions
	if e.Total == 0 {
		return errors.New("Add at least one repetition")
	}
	if len(e.Note) > 2000 {
		return errors.New("Notes must be at most 2,000 bytes")
	}
	t, err := time.Parse(time.RFC3339, e.OccurredAt)
	if err != nil {
		return errors.New("A date and time with UTC offset is required")
	}
	loc, err := time.LoadLocation(e.TimeZone)
	if err != nil || e.TimeZone == "Local" || e.TimeZone == "" {
		return errors.New("A valid IANA time zone is required")
	}
	_, offset := t.Zone()
	_, expected := t.In(loc).Zone()
	if offset != expected {
		return errors.New("The time offset does not match the time zone on this date")
	}
	if t.Year() < 1900 || t.After(time.Now().Add(5*time.Minute)) {
		return errors.New("Use a date between 1900 and now")
	}
	e.OffsetMinutes = offset / 60
	return nil
}
func serialized(e *entry) any {
	if e == nil {
		return nil
	}
	b, _ := json.Marshal(e)
	return string(b)
}
func (a *app) mutate(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userKey).(user)
	var e entry
	if r.Method != "DELETE" {
		if !decode(w, r, &e) {
			return
		}
		if err := validate(&e); err != nil {
			fail(w, 422, err.Error())
			return
		}
	}
	requestKey := r.Header.Get("Idempotency-Key")
	if len(requestKey) > 128 {
		fail(w, 400, "Idempotency key must be at most 128 bytes")
		return
	}
	requestHash := ""
	if r.Method == "POST" && requestKey != "" {
		requestHash = digest(serialized(&e).(string))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback()
	var before, after *entry
	action := "created"
	status := 200
	if r.Method == "POST" {
		if requestKey != "" {
			var previousHash, body string
			err = tx.QueryRow(`SELECT request_hash,entry_body FROM create_requests WHERE user_id=? AND request_key=?`, u.ID, requestKey).Scan(&previousHash, &body)
			if err == nil {
				if previousHash != requestHash {
					fail(w, 409, "This idempotency key was already used for another entry")
					return
				}
				var previous entry
				if err = json.Unmarshal([]byte(body), &previous); err != nil {
					internal(w, err)
					return
				}
				respond(w, 201, map[string]any{"entry": previous})
				return
			}
			if !errors.Is(err, sql.ErrNoRows) {
				internal(w, err)
				return
			}
		}

		e.ID = token()
		e.UserID = u.ID
		e.CreatedAt = now()
		e.UpdatedAt = e.CreatedAt
		e.Version = 1
		after = &e
		status = 201
		if _, err = tx.Exec(`INSERT INTO entries(id,user_id,body,occurred_at) VALUES(?,?,?,?)`, e.ID, u.ID, serialized(&e), mustUTC(e.OccurredAt)); err != nil {
			internal(w, err)
			return
		}
	} else {
		var body string
		old := entry{}
		err = tx.QueryRow(`SELECT body FROM entries WHERE id=? AND user_id=? AND deleted=0`, r.PathValue("id"), u.ID).Scan(&body)
		if errors.Is(err, sql.ErrNoRows) {
			fail(w, 404, "Entry not found")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
		if err = json.Unmarshal([]byte(body), &old); err != nil {
			internal(w, err)
			return
		}
		before = &old
		if r.Method == "DELETE" {
			v, _ := strconv.Atoi(r.URL.Query().Get("version"))
			if v != old.Version {
				fail(w, 409, "This entry changed. Refresh and try again.")
				return
			}
			e = old
			action = "deleted"
			_, err = tx.Exec(`UPDATE entries SET deleted=1 WHERE id=? AND user_id=?`, old.ID, u.ID)
		} else {
			if e.Version != old.Version {
				fail(w, 409, "This entry changed. Refresh and try again.")
				return
			}
			e.ID = old.ID
			e.UserID = u.ID
			e.Initial = old.Initial
			e.CreatedAt = old.CreatedAt
			e.UpdatedAt = now()
			e.Version = old.Version + 1
			after = &e
			action = "updated"
			_, err = tx.Exec(`UPDATE entries SET body=?,occurred_at=? WHERE id=? AND user_id=?`, serialized(&e), mustUTC(e.OccurredAt), e.ID, u.ID)
		}
		if err != nil {
			internal(w, err)
			return
		}
	}
	if r.Method == "POST" && requestKey != "" {
		if _, err = tx.Exec(`INSERT INTO create_requests(user_id,request_key,request_hash,entry_body) VALUES(?,?,?,?)`, u.ID, requestKey, requestHash, serialized(after)); err != nil {
			internal(w, err)
			return
		}
	}
	if _, err = tx.Exec(`INSERT INTO audit(id,user_id,entry_id,action,at,before_body,after_body) VALUES(?,?,?,?,?,?,?)`, token(), u.ID, e.ID, action, now(), serialized(before), serialized(after)); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internal(w, err)
		return
	}
	respond(w, status, map[string]any{"entry": after})
}
func mustUTC(s string) string {
	t, _ := time.Parse(time.RFC3339, s)
	return t.UTC().Format(time.RFC3339)
}
func (a *app) listAudit(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userKey).(user)
	rows, err := a.db.Query(`SELECT id,entry_id,action,at,before_body,after_body FROM audit WHERE user_id=? ORDER BY at DESC`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []audit{}
	for rows.Next() {
		var v audit
		var before, after sql.NullString
		if err = rows.Scan(&v.ID, &v.EntryID, &v.Action, &v.At, &before, &after); err != nil {
			internal(w, err)
			return
		}
		if before.Valid {
			if err = json.Unmarshal([]byte(before.String), &v.Before); err != nil {
				internal(w, err)
				return
			}
		}
		if after.Valid {
			if err = json.Unmarshal([]byte(after.String), &v.After); err != nil {
				internal(w, err)
				return
			}
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, out)
}
func getenv(k, v string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return v
}
func main() {
	mockMode := flag.Bool("mock", false, "Run a loopback-only mock API with disposable fixtures")
	flag.Parse()
	if *mockMode {
		if err := runMock(); err != nil {
			log.Fatal(err)
		}
		return
	}
	production := os.Getenv("APP_ENV") == "production"
	secret := os.Getenv("SESSION_SECRET")
	mode := getenv("EMAIL_MODE", "log")
	origin := getenv("APP_ORIGIN", "http://127.0.0.1:5173")
	if mode != "log" && mode != "resend" {
		log.Fatal("EMAIL_MODE must be log or resend")
	}
	if production && (len(secret) < 32 || mode != "resend" || !strings.HasPrefix(origin, "https://")) {
		log.Fatal("Production requires a 32-character SESSION_SECRET, EMAIL_MODE=resend and an HTTPS APP_ORIGIN")
	}
	if mode == "resend" && (os.Getenv("RESEND_API_KEY") == "" || os.Getenv("EMAIL_FROM") == "") {
		log.Fatal("RESEND_API_KEY and EMAIL_FROM are required")
	}
	if secret == "" {
		secret = token()
	}
	if _, err := url.ParseRequestURI(origin); err != nil {
		log.Fatal("Invalid APP_ORIGIN")
	}
	path := getenv("DATABASE_PATH", "data/karma.db")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		log.Fatal(err)
	}
	db, err := openDB(path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	_ = os.Chmod(path, 0600)
	a := &app{db: db, secret: secret, production: production, emailMode: mode, origin: origin}
	a.send = func(ctx context.Context, email, code string) error {
		if mode == "log" {
			log.Printf("DEVELOPMENT ONLY — sign-in code for %s: %s", email, code)
			return nil
		}
		body, _ := json.Marshal(map[string]any{"from": os.Getenv("EMAIL_FROM"), "to": []string{email}, "subject": "Your Karma sign-in code", "text": "Your sign-in code is " + code + ". It expires in 10 minutes. If you did not request it, ignore this email."})
		req, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+os.Getenv("RESEND_API_KEY"))
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("email service returned %d", resp.StatusCode)
		}
		return nil
	}
	addr := getenv("LISTEN_ADDR", "127.0.0.1:8080")
	if mode == "log" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			log.Fatal("Log email mode requires a loopback LISTEN_ADDR")
		}
	}
	server := &http.Server{Addr: addr, Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("Karma API listening at http://%s (email mode: %s)", addr, mode)
	log.Fatal(server.ListenAndServe())
}
