package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"
)

type entryCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

// Keyset pagination remains stable when new entries are inserted before the current page.
func (a *app) pageEntries(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userKey).(user)
	query := `SELECT body,occurred_at,id FROM entries WHERE user_id=? AND deleted=0`
	args := []any{u.ID}
	if practice := r.URL.Query().Get("practice"); practice != "" {
		if !practices[practice] {
			respond(w, 400, map[string]string{"error": "Invalid practice"})
			return
		}
		query += ` AND json_extract(body,'$.practice')=?`
		args = append(args, practice)
	}
	if since := r.URL.Query().Get("since"); since != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			respond(w, 400, map[string]string{"error": "Invalid period"})
			return
		}
		query += ` AND occurred_at>=?`
		args = append(args, parsed.UTC().Format(time.RFC3339))
	}
	if encoded := r.URL.Query().Get("cursor"); encoded != "" {
		var cursor entryCursor
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.At == "" || cursor.ID == "" {
			respond(w, 400, map[string]string{"error": "Invalid cursor"})
			return
		}
		if _, err = time.Parse(time.RFC3339, cursor.At); err != nil {
			respond(w, 400, map[string]string{"error": "Invalid cursor"})
			return
		}
		query += ` AND (occurred_at<? OR (occurred_at=? AND id<?))`
		args = append(args, cursor.At, cursor.At, cursor.ID)
	}
	const pageSize = 20
	query += ` ORDER BY occurred_at DESC,id DESC LIMIT ?`
	args = append(args, pageSize+1)
	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []entry{}
	last := entryCursor{}
	hasMore := false
	for rows.Next() {
		var body, at, id string
		if err = rows.Scan(&body, &at, &id); err != nil {
			internal(w, err)
			return
		}
		if len(out) == pageSize {
			hasMore = true
			break
		}
		var e entry
		if err = json.Unmarshal([]byte(body), &e); err != nil {
			internal(w, err)
			return
		}
		out = append(out, e)
		last = entryCursor{at, id}
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	next := ""
	if hasMore {
		data, _ := json.Marshal(last)
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	respond(w, 200, map[string]any{"items": out, "nextCursor": next})
}

// Daily aggregates contain no notes or individual records. Full records are fetched by the feed.
func (a *app) practiceSummary(w http.ResponseWriter, r *http.Request) {
	zone, err := time.LoadLocation(r.URL.Query().Get("timeZone"))
	if err != nil {
		respond(w, 400, map[string]string{"error": "Invalid time zone"})
		return
	}
	u := r.Context().Value(userKey).(user)
	rows, err := a.db.QueryContext(r.Context(), `SELECT body FROM entries WHERE user_id=? AND deleted=0`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	groups := map[string]*entry{}
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
		at, err := time.Parse(time.RFC3339, e.OccurredAt)
		if err != nil {
			internal(w, err)
			return
		}
		key := e.Practice + ":" + at.In(zone).Format("2006-01-02")
		if e.Initial {
			key = e.Practice + ":initial"
		}
		if group := groups[key]; group != nil {
			group.Total += e.Total
		} else {
			groups[key] = &entry{ID: key, Practice: e.Practice, Total: e.Total, Initial: e.Initial, OccurredAt: e.OccurredAt, TimeZone: zone.String()}
		}
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	out := []entry{}
	for _, group := range groups {
		out = append(out, *group)
	}
	respond(w, 200, out)
}
