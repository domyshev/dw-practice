package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const mockLoginCode = "123456"
const mockDemoEmail = "demo@dwpractice.test"

//go:embed mockdata/seed.json
var mockSeed []byte

type mockEntry struct {
	ID              string `json:"id"`
	Practice        string `json:"practice"`
	Large           int    `json:"large"`
	Small           int    `json:"small"`
	Repetitions     int    `json:"repetitions"`
	Initial         bool   `json:"initial"`
	DaysAgo         int    `json:"daysAgo"`
	MinutesAgo      int    `json:"minutesAgo"`
	TimeZone        string `json:"timeZone"`
	Note            string `json:"note"`
	EditRepetitions *int   `json:"editRepetitions"`
	Deleted         bool   `json:"deleted"`
}

type mockFixtures struct {
	Users []struct {
		ID      string      `json:"id"`
		Email   string      `json:"email"`
		Entries []mockEntry `json:"entries"`
	} `json:"users"`
}

func (a *app) sessionCookieName() string {
	if a.mock {
		return "dw_mock_session"
	}
	return "karma_session"
}

func newMockApp(origin string) (*app, error) {
	db, err := openDB(":memory:")
	if err != nil {
		return nil, err
	}
	a := &app{db: db, secret: token(), emailMode: "log", origin: origin, mock: true}
	a.send = func(_ context.Context, email, code string) error {
		log.Printf("MOCK sign-in for %s: %s (no email sent)", email, code)
		return nil
	}
	if err = a.resetMockData(); err != nil {
		db.Close()
		return nil, err
	}
	return a, nil
}

func mockAudit(tx *sql.Tx, userID, entryID, action, at string, before, after *entry) error {
	_, err := tx.Exec(`INSERT INTO audit(id,user_id,entry_id,action,at,before_body,after_body) VALUES(?,?,?,?,?,?,?)`, token(), userID, entryID, action, at, serialized(before), serialized(after))
	return err
}

func (a *app) resetMockData() error {
	if !a.mock || a.production {
		return errors.New("mock reset is unavailable outside mock mode")
	}
	var fixtures mockFixtures
	if err := json.Unmarshal(mockSeed, &fixtures); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// This transaction operates only on a disposable in-memory mock database.
	_, err = tx.Exec(`DROP TRIGGER audit_no_delete; DELETE FROM create_requests; DELETE FROM audit; DELETE FROM entries; DELETE FROM sessions; DELETE FROM codes; DELETE FROM users;
 CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit BEGIN SELECT RAISE(ABORT,'Audit records are immutable'); END;`)
	if err != nil {
		return err
	}
	anchor := time.Now().UTC().Truncate(time.Second)
	for _, u := range fixtures.Users {
		if _, err = tx.Exec(`INSERT INTO users(id,email,created_at) VALUES(?,?,?)`, u.ID, u.Email, anchor.AddDate(0, 0, -200).Format(time.RFC3339)); err != nil {
			return err
		}
		for _, fixture := range u.Entries {
			zone := fixture.TimeZone
			if zone == "" {
				zone = "Europe/Madrid"
			}
			loc, err := time.LoadLocation(zone)
			if err != nil {
				return err
			}
			occurred := anchor.AddDate(0, 0, -fixture.DaysAgo).Add(-time.Duration(fixture.MinutesAgo) * time.Minute).In(loc)
			e := entry{ID: fixture.ID, UserID: u.ID, Practice: fixture.Practice, Large: fixture.Large, Small: fixture.Small, Repetitions: fixture.Repetitions, Initial: fixture.Initial, OccurredAt: occurred.Format(time.RFC3339), TimeZone: zone, Note: fixture.Note, CreatedAt: occurred.UTC().Format(time.RFC3339), UpdatedAt: occurred.UTC().Format(time.RFC3339), Version: 1}
			if err = validate(&e); err != nil {
				return fmt.Errorf("fixture %s: %w", e.ID, err)
			}
			if _, err = tx.Exec(`INSERT INTO entries(id,user_id,body,occurred_at) VALUES(?,?,?,?)`, e.ID, u.ID, serialized(&e), mustUTC(e.OccurredAt)); err != nil {
				return err
			}
			if err = mockAudit(tx, u.ID, e.ID, "created", e.CreatedAt, nil, &e); err != nil {
				return err
			}
			if fixture.EditRepetitions != nil {
				before := e
				e.Repetitions = *fixture.EditRepetitions
				e.Note = "Corrected count — example edit"
				e.Version++
				e.UpdatedAt = anchor.Add(-time.Minute).Format(time.RFC3339)
				if err = validate(&e); err != nil {
					return err
				}
				if _, err = tx.Exec(`UPDATE entries SET body=? WHERE id=?`, serialized(&e), e.ID); err != nil {
					return err
				}
				if err = mockAudit(tx, u.ID, e.ID, "updated", e.UpdatedAt, &before, &e); err != nil {
					return err
				}
			}
			if fixture.Deleted {
				if _, err = tx.Exec(`UPDATE entries SET deleted=1 WHERE id=?`, e.ID); err != nil {
					return err
				}
				if err = mockAudit(tx, u.ID, e.ID, "deleted", anchor.Add(-30*time.Second).Format(time.RFC3339), &e, nil); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func (a *app) resetMock(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decode(w, r, &input) {
		return
	}
	if err := a.resetMockData(); err != nil {
		internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.sessionCookieName(), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	respond(w, 200, map[string]any{"ok": true, "mock": true, "message": "Fixtures restored; sign in again"})
}

func mockAddress(port string) (string, error) {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("MOCK_API_PORT must be between 1 and 65535")
	}
	return net.JoinHostPort("127.0.0.1", port), nil
}

func runMock() error {
	if os.Getenv("APP_ENV") == "production" {
		return errors.New("mock mode is not allowed in production")
	}
	addr, err := mockAddress(getenv("MOCK_API_PORT", "8081"))
	if err != nil {
		return err
	}
	origin := getenv("MOCK_APP_ORIGIN", "http://127.0.0.1:5174")
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		return errors.New("MOCK_APP_ORIGIN must be an HTTP loopback origin")
	}
	a, err := newMockApp(origin)
	if err != nil {
		return err
	}
	defer a.db.Close()
	server := &http.Server{Addr: addr, Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("MOCK API: http://%s | demo: %s | code: %s | disposable in-memory data", addr, mockDemoEmail, mockLoginCode)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Mock-only login deliberately accepts any nonempty password. No password is stored.
func (a *app) mockPasswordLogin(w http.ResponseWriter, r *http.Request) {
	if !a.mock || a.production {
		fail(w, 404, "Endpoint not found")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &input) {
		return
	}
	email, err := normalize(input.Email)
	if err != nil {
		fail(w, 422, "Enter a valid email address")
		return
	}
	if input.Password == "" {
		fail(w, 422, "Enter any password for mock login")
		return
	}
	a.mockSignIn(w, r, email)
}

func (a *app) mockSignIn(w http.ResponseWriter, r *http.Request, email string) {
	if !a.mock || a.production {
		fail(w, 404, "Endpoint not found")
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
	if err = tx.Commit(); err != nil {
		internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.sessionCookieName(), Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 86400})
	respond(w, 200, u)
}
