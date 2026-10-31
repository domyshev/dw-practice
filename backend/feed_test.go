package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func TestEntryFeedPaginationAndIsolation(t *testing.T) {
	_, h, codes := fixture(t)
	cookie := login(t, h, codes, "feed@example.com")
	other := login(t, h, codes, "other@example.com")
	for i := 0; i < 45; i++ {
		e := validEntry()
		e.Note = fmt.Sprint(i)
		create(t, h, cookie, e)
	}
	create(t, h, other, validEntry())
	type page struct {
		Items []entry `json:"items"`
		Next  string  `json:"nextCursor"`
	}
	load := func(path string, c *http.Cookie) page {
		t.Helper()
		w := call(h, "GET", path, nil, c)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var p page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := load("/api/entries/page", cookie)
	if len(first.Items) != 20 || first.Next == "" {
		t.Fatal("Expected first bounded page")
	}
	// An insertion at the head cannot shift already-read cursor boundaries.
	newer := validEntry()
	newer.OccurredAt = "2026-02-01T12:00:00+01:00"
	create(t, h, cookie, newer)
	seen := map[string]bool{}
	for _, e := range first.Items {
		seen[e.ID] = true
	}
	cursor := first.Next
	for cursor != "" {
		p := load("/api/entries/page?cursor="+url.QueryEscape(cursor), cookie)
		for _, e := range p.Items {
			if seen[e.ID] {
				t.Fatal("Duplicate entry")
			}
			seen[e.ID] = true
		}
		cursor = p.Next
	}
	if len(seen) != 45 {
		t.Fatalf("Expected original 45 entries, got %d", len(seen))
	}
	if len(load("/api/entries/page?practice=prostrations", cookie).Items) != 0 {
		t.Fatal("Practice filter ignored")
	}
	if len(load("/api/entries/page?since=2026-02-01T00%3A00%3A00Z", cookie).Items) != 1 {
		t.Fatal("Period filter ignored")
	}
	if len(load("/api/entries/page", other).Items) != 1 {
		t.Fatal("Account isolation failed")
	}
	for _, query := range []string{"cursor=invalid", "since=bad", "practice=bad"} {
		w := call(h, "GET", "/api/entries/page?"+query, nil, cookie)
		if w.Code != 400 {
			t.Fatal("Invalid query accepted", query)
		}
	}
	if w := call(h, "GET", "/api/entries/page", nil, nil); w.Code != 401 {
		t.Fatal("Feed must require authentication")
	}
}

func TestSummaryAggregatesDeviceDaysWithoutRecordDetails(t *testing.T) {
	_, h, codes := fixture(t)
	cookie := login(t, h, codes, "summary@example.com")
	e := validEntry()
	e.OccurredAt = "2026-01-21T00:30:00+01:00"
	a := create(t, h, cookie, e)
	e.OccurredAt = "2026-01-20T20:30:00+01:00"
	create(t, h, cookie, e)
	e.Initial = true
	create(t, h, cookie, e)
	w := call(h, "GET", "/api/summary?timeZone=America%2FNew_York", nil, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var groups []entry
	json.Unmarshal(w.Body.Bytes(), &groups)
	if len(groups) != 2 {
		t.Fatalf("Expected daily aggregate and separate starting balance, got %d", len(groups))
	}
	for _, g := range groups {
		if g.Note != "" || g.UserID != "" || g.Version != 0 {
			t.Fatal("Summary leaked individual record details")
		}
		if !g.Initial && g.Total != 2*a.Total {
			t.Fatal("Incorrect day total")
		}
		if g.Initial && g.Total != a.Total {
			t.Fatal("Incorrect starting balance")
		}
	}
	if w := call(h, "GET", "/api/summary?timeZone=invalid", nil, cookie); w.Code != 400 {
		t.Fatal("Invalid zone accepted")
	}
}

func TestMockThousandEntryFeedReachesCompleteFinalPage(t *testing.T) {
	_, h := mockFixture(t)
	cookie := mockLogin(t, h, mockDemoEmail)
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		w := call(h, "GET", "/api/entries/page?cursor="+url.QueryEscape(cursor), nil, cookie)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var page struct {
			Items []entry `json:"items"`
			Next  string  `json:"nextCursor"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 20 {
			t.Fatal("Unbounded page")
		}
		for _, e := range page.Items {
			if seen[e.ID] {
				t.Fatal("Duplicate", e.ID)
			}
			seen[e.ID] = true
		}
		pages++
		cursor = page.Next
		if cursor == "" {
			if len(page.Items) != 14 {
				t.Fatalf("Final page should contain 14 records, got %d", len(page.Items))
			}
			break
		}
		if pages > 51 {
			t.Fatal("Cursor did not terminate")
		}
	}
	if len(seen) != 1014 || pages != 51 {
		t.Fatalf("Got %d records in %d pages", len(seen), pages)
	}
}
