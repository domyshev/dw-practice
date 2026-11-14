package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const postgresSchema = `
CREATE TABLE IF NOT EXISTS schema_version(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL,imported_at TEXT);
CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,email TEXT UNIQUE NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS codes(id BIGSERIAL PRIMARY KEY,email TEXT NOT NULL,hash TEXT NOT NULL,ip TEXT NOT NULL,created_at BIGINT NOT NULL,expires_at BIGINT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,consumed INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS codes_email ON codes(email,created_at);
CREATE INDEX IF NOT EXISTS codes_ip ON codes(ip,created_at);
CREATE TABLE IF NOT EXISTS sessions(hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),expires_at BIGINT NOT NULL);
CREATE TABLE IF NOT EXISTS entries(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),body TEXT NOT NULL,occurred_at TEXT NOT NULL,deleted INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS entries_user ON entries(user_id,deleted);
CREATE INDEX IF NOT EXISTS entries_page ON entries(user_id,deleted,occurred_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS create_requests(user_id TEXT NOT NULL REFERENCES users(id),request_key TEXT NOT NULL,request_hash TEXT NOT NULL,entry_body TEXT NOT NULL,PRIMARY KEY(user_id,request_key));
CREATE TABLE IF NOT EXISTS audit(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),entry_id TEXT NOT NULL,action TEXT NOT NULL,at TEXT NOT NULL,before_body TEXT,after_body TEXT);
CREATE INDEX IF NOT EXISTS audit_user ON audit(user_id,at);
CREATE OR REPLACE FUNCTION reject_audit_change() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'Audit records are immutable'; END $$;
DROP TRIGGER IF EXISTS audit_immutable ON audit;
CREATE TRIGGER audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON audit FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_change();
INSERT INTO schema_version(id,version) VALUES(1,1) ON CONFLICT(id) DO NOTHING;
`

func openPostgres(dsn string) (*sql.DB, error) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return nil, errors.New("DATABASE_URL must be a PostgreSQL URL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("unable to connect to PostgreSQL")
	}
	return db, nil
}

// Both PostgreSQL and SQLite accept numbered parameters. Dynamic feed queries
// contain no literal question marks and use this shared conversion.
func bindSQL(query string) string {
	n := 0
	return replaceParameters(query, &n)
}
func replaceParameters(query string, n *int) string {
	var out strings.Builder
	for _, r := range query {
		if r == '?' {
			*n++
			out.WriteString("$" + strconv.Itoa(*n))
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}
func (a *app) databaseName() string {
	if a.postgres {
		return "postgresql"
	}
	return "sqlite"
}
func (a *app) lockLocal() func() {
	if a.postgres {
		return func() {}
	}
	a.mu.Lock()
	return a.mu.Unlock
}

// Transaction locks coordinate OTP limits, code consumption and same-user
// journal mutations across all app instances, including idempotent retries.
func (a *app) lockTransaction(tx *sql.Tx, keys ...string) error {
	if !a.postgres {
		return nil
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
			return err
		}
	}
	return nil
}

type migrationTable struct{ name, columns, order string }

var migrationTables = []migrationTable{
	{"users", "id,email,created_at", "id"},
	{"codes", "id,email,hash,ip,created_at,expires_at,attempts,consumed", "id"},
	{"sessions", "hash,user_id,expires_at", "hash"},
	{"entries", "id,user_id,body,occurred_at,deleted", "id"},
	{"create_requests", "user_id,request_key,request_hash,entry_body", "user_id,request_key"},
	{"audit", "id,user_id,entry_id,action,at,before_body,after_body", "id"},
}

func scanValues(rows *sql.Rows, count int) ([]any, error) {
	values := make([]any, count)
	dest := make([]any, count)
	for i := range values {
		dest[i] = &values[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	for i, v := range values {
		if b, ok := v.([]byte); ok {
			values[i] = string(b)
		}
	}
	return values, nil
}
func tableFingerprint(tx *sql.Tx, table migrationTable) (string, int, error) {
	rows, err := tx.Query("SELECT " + table.columns + " FROM " + table.name + " ORDER BY " + table.order)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	h := sha256.New()
	n := 0
	for rows.Next() {
		v, err := scanValues(rows, len(strings.Split(table.columns, ",")))
		if err != nil {
			return "", 0, err
		}
		if err = json.NewEncoder(h).Encode(v); err != nil {
			return "", 0, err
		}
		n++
	}
	return fmt.Sprintf("%x", h.Sum(nil)), n, rows.Err()
}

func migratePostgres(source string) error {
	db, err := openPostgres(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(74198762001)`); err != nil {
		return err
	}
	if _, err = tx.Exec(postgresSchema); err != nil {
		return fmt.Errorf("initialize PostgreSQL schema: %w", err)
	}
	if source == "" {
		return tx.Commit()
	}
	var imported sql.NullString
	if err = tx.QueryRow(`SELECT imported_at FROM schema_version WHERE id=1`).Scan(&imported); err != nil {
		return err
	}
	if imported.Valid {
		return errors.New("SQLite import has already completed; refusing to import twice")
	}
	for _, table := range migrationTables {
		var count int
		if err = tx.QueryRow("SELECT count(*) FROM " + table.name).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("destination PostgreSQL database must be empty")
		}
	}
	path, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	src, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String())
	if err != nil {
		return err
	}
	defer src.Close()
	src.SetMaxOpenConns(1)
	read, err := src.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer read.Rollback()
	var check string
	if err = read.QueryRow(`PRAGMA quick_check`).Scan(&check); err != nil || check != "ok" {
		return errors.New("source SQLite integrity check failed")
	}
	violations, err := read.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	hasViolation := violations.Next()
	err = violations.Err()
	violations.Close()
	if err != nil || hasViolation {
		return errors.New("source SQLite foreign key check failed")
	}
	for _, table := range migrationTables {
		rows, err := read.Query("SELECT " + table.columns + " FROM " + table.name + " ORDER BY " + table.order)
		if err != nil {
			return err
		}
		count := len(strings.Split(table.columns, ","))
		marks := make([]string, count)
		for i := range marks {
			marks[i] = "$" + strconv.Itoa(i+1)
		}
		statement := "INSERT INTO " + table.name + "(" + table.columns + ") VALUES(" + strings.Join(marks, ",") + ")"
		for rows.Next() {
			v, e := scanValues(rows, count)
			if e != nil {
				rows.Close()
				return e
			}
			if _, e = tx.Exec(statement, v...); e != nil {
				rows.Close()
				return fmt.Errorf("import table %s: %w", table.name, e)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		srcHash, srcCount, err := tableFingerprint(read, table)
		if err != nil {
			return err
		}
		dstHash, dstCount, err := tableFingerprint(tx, table)
		if err != nil {
			return err
		}
		if srcHash != dstHash || srcCount != dstCount {
			return fmt.Errorf("verification failed for %s", table.name)
		}
		log.Printf("Verified import table %s: %d rows, SHA-256 %s", table.name, dstCount, dstHash)
	}
	if _, err = tx.Exec(`SELECT setval(pg_get_serial_sequence('codes','id'),COALESCE((SELECT max(id) FROM codes),1),(SELECT count(*)>0 FROM codes))`); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE schema_version SET imported_at=$1 WHERE id=1`, now()); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	log.Print("SQLite import committed and verified; retain the original snapshot for rollback")
	return nil
}
