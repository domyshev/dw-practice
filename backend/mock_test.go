package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func mockFixture(t *testing.T) (*app, http.Handler) {
	t.Helper()
	a, err := newMockApp("http://127.0.0.1:5174")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.db.Close() })
	return a, a.handler()
}
func mockLogin(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	if w := call(h, "POST", "/api/auth/request", map[string]string{"email": email}, nil); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := call(h, "POST", "/api/auth/verify", map[string]string{"email": email, "code": mockLoginCode}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}
func mockEntries(t *testing.T, h http.Handler, c *http.Cookie) []entry {
	t.Helper()
	w := call(h, "GET", "/api/entries", nil, c)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var result []entry
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMockHealthAndSeededJournal(t *testing.T) {
	_, h := mockFixture(t)
	var health map[string]any
	_ = json.Unmarshal(call(h, "GET", "/api/health", nil, nil).Body.Bytes(), &health)
	if health["mock"] != true || health["loginCode"] != mockLoginCode || health["demoEmail"] != mockDemoEmail {
		t.Fatal(health)
	}
	c := mockLogin(t, h, mockDemoEmail)
	entries := mockEntries(t, h, c)
	if len(entries) != 14 {
		t.Fatal("expected 14 active demo entries", len(entries))
	}
	seen := map[string]bool{}
	initial := 0
	fractional, edited := false, false
	for _, e := range entries {
		if e.UserID != "mock-demo" {
			t.Fatal("foreign seed entry", e.ID)
		}
		seen[e.Practice] = true
		if e.Initial {
			initial++
		}
		if e.OffsetMinutes == 345 {
			fractional = true
		}
		if e.ID == "demo-edited" && e.Version == 2 && e.Repetitions == 15 {
			edited = true
		}
		if err := validate(&e); err != nil {
			t.Fatal(e.ID, err)
		}
	}
	if len(seen) != 4 || initial != 4 || !fractional || !edited {
		t.Fatal("missing seed scenario", seen, initial, fractional, edited)
	}
	var history []audit
	_ = json.Unmarshal(call(h, "GET", "/api/audit", nil, c).Body.Bytes(), &history)
	if len(history) != 17 {
		t.Fatal("expected creation/update/deletion events", len(history))
	}
	actions := map[string]bool{}
	for _, event := range history {
		actions[event.Action] = true
	}
	if !actions["created"] || !actions["updated"] || !actions["deleted"] {
		t.Fatal(actions)
	}
}

func TestMockAccountIsolationAndEmptyAccount(t *testing.T) {
	_, h := mockFixture(t)
	demo := mockLogin(t, h, mockDemoEmail)
	second := mockLogin(t, h, "second@dwpractice.test")
	empty := mockLogin(t, h, "empty@dwpractice.test")
	if len(mockEntries(t, h, demo)) != 14 || len(mockEntries(t, h, second)) != 2 || len(mockEntries(t, h, empty)) != 0 {
		t.Fatal("incorrect account fixtures")
	}
	for _, e := range mockEntries(t, h, second) {
		if e.UserID != "mock-second" {
			t.Fatal("demo journal leaked into second account")
		}
	}
	w := call(h, "DELETE", "/api/entries/demo-today-prostrations?version=1", nil, second)
	if w.Code != 404 {
		t.Fatal("cross-user deletion allowed", w.Code)
	}
}

func TestMockCodeAlwaysWorksWithoutRequest(t *testing.T) {
	_, h := mockFixture(t)
	input := map[string]string{"email": mockDemoEmail, "code": mockLoginCode}
	for i := 0; i < 3; i++ {
		w := call(h, "POST", "/api/auth/verify", input, nil)
		if w.Code != 200 {
			t.Fatal("fixed code must always work", w.Code, w.Body.String())
		}
		c := w.Result().Cookies()[0]
		if c.Name != "dw_mock_session" {
			t.Fatal("mock cookie conflicts with real server", c.Name)
		}
		realCookie := &http.Cookie{Name: "karma_session", Value: c.Value}
		if call(h, "GET", "/api/me", nil, realCookie).Code != 401 {
			t.Fatal("real cookie namespace accepted by mock")
		}
	}
	for _, code := range []string{"999999", "000000", "654321"} {
		if call(h, "POST", "/api/auth/verify", map[string]string{"email": mockDemoEmail, "code": code}, nil).Code != 200 {
			t.Fatal("valid mock code rejected", code)
		}
	}
	for _, code := range []string{"123", "abcdef"} {
		if call(h, "POST", "/api/auth/verify", map[string]string{"email": mockDemoEmail, "code": code}, nil).Code != 401 {
			t.Fatal("malformed mock code accepted", code)
		}
	}
}

func TestMockPasswordLoginWithArbitraryCredentials(t *testing.T) {
	_, h := mockFixture(t)
	var firstID string
	for _, password := range []string{"anything", "a different password"} {
		w := call(h, "POST", "/api/mock/login", map[string]string{"email": "local+agent@example.test", "password": password}, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var u user
		_ = json.Unmarshal(w.Body.Bytes(), &u)
		if u.Email != "local+agent@example.test" {
			t.Fatal(u)
		}
		if firstID != "" && u.ID != firstID {
			t.Fatal("password variations created another account")
		}
		firstID = u.ID
		if call(h, "GET", "/api/me", nil, w.Result().Cookies()[0]).Code != 200 {
			t.Fatal("mock password session not recognized")
		}
	}
	if call(h, "POST", "/api/mock/login", map[string]string{"email": "bad", "password": "any"}, nil).Code != 422 {
		t.Fatal("invalid email accepted")
	}
	if call(h, "POST", "/api/mock/login", map[string]string{"email": mockDemoEmail, "password": ""}, nil).Code != 422 {
		t.Fatal("empty password accepted")
	}
}

func TestMockResetRestoresFixturesAndInvalidatesSessions(t *testing.T) {
	a, h := mockFixture(t)
	c := mockLogin(t, h, mockDemoEmail)
	e := create(t, h, c, validEntry())
	if len(mockEntries(t, h, c)) != 15 {
		t.Fatal("entry was not created")
	}
	if w := call(h, "POST", "/api/mock/reset", map[string]any{}, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if call(h, "GET", "/api/me", nil, c).Code != 401 {
		t.Fatal("reset did not revoke sessions")
	}
	c = mockLogin(t, h, mockDemoEmail)
	entries := mockEntries(t, h, c)
	if len(entries) != 14 {
		t.Fatal("fixtures not restored")
	}
	for _, v := range entries {
		if v.ID == e.ID {
			t.Fatal("custom entry survived reset")
		}
	}
	if _, err := a.db.Exec(`DELETE FROM audit`); err == nil {
		t.Fatal("reset left audit deletion protection disabled")
	}
}

func TestMockRoutesAndCodeAreNotExposedByNormalServer(t *testing.T) {
	_, h, _ := fixture(t)
	w := call(h, "GET", "/api/health", nil, nil)
	var health map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &health)
	for _, key := range []string{"mock", "loginCode", "demoEmail"} {
		if _, exists := health[key]; exists {
			t.Fatal("mock metadata exposed by ordinary API", key)
		}
	}
	if call(h, "POST", "/api/mock/login", map[string]string{"email": mockDemoEmail, "password": "any"}, nil).Code != 404 {
		t.Fatal("mock password login exposed outside mock mode")
	}
	if call(h, "POST", "/api/auth/verify", map[string]string{"email": mockDemoEmail, "code": mockLoginCode}, nil).Code != 401 {
		t.Fatal("normal server accepted fixed mock code")
	}
	if call(h, "POST", "/api/mock/reset", map[string]any{}, nil).Code != 404 {
		t.Fatal("reset route exposed outside mock mode")
	}
}

func TestMockResetRejectsForeignOrigin(t *testing.T) {
	_, h := mockFixture(t)
	r := httptest.NewRequest("POST", "/api/mock/reset", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign origin could reset mock state", w.Code)
	}
}

func TestMockDoesNotTouchConfiguredDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real.db")
	t.Setenv("DATABASE_PATH", path)
	_, _ = mockFixture(t)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("mock touched the configured database", err)
	}
}

func TestMockProductionAndPortGuards(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	if err := runMock(); err == nil {
		t.Fatal("mock mode allowed in production")
	}
	for _, port := range []string{"0", "65536", "invalid", "0.0.0.0:8081"} {
		if _, err := mockAddress(port); err == nil {
			t.Fatal("unsafe mock port accepted", port)
		}
	}
	if addr, err := mockAddress("8081"); err != nil || addr != "127.0.0.1:8081" {
		t.Fatal(addr, err)
	}
}
