package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesExpectedSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wacli.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	cols, err := tableColumns(db.sql, "messages")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}

	for _, want := range []string{
		"chat_name",
		"sender_name",
		"display_text",
		"message_kind",
		"raw_summary",
		"local_path",
		"downloaded_at",
	} {
		if !cols[want] {
			t.Fatalf("expected messages column %q to exist", want)
		}
	}
}

func TestOpenMigratesMessageKindAndRawSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wacli.db")

	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		);

		CREATE TABLE messages (
			rowid INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_jid TEXT NOT NULL,
			chat_name TEXT,
			msg_id TEXT NOT NULL,
			sender_jid TEXT,
			sender_name TEXT,
			ts INTEGER NOT NULL,
			from_me INTEGER NOT NULL,
			text TEXT,
			display_text TEXT,
			media_type TEXT,
			media_caption TEXT,
			filename TEXT,
			mime_type TEXT,
			direct_path TEXT,
			media_key BLOB,
			file_sha256 BLOB,
			file_enc_sha256 BLOB,
			file_length INTEGER,
			local_path TEXT,
			downloaded_at INTEGER,
			UNIQUE(chat_jid, msg_id)
		);

		CREATE TABLE chats (
			jid TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			name TEXT,
			last_message_ts INTEGER
		);

		INSERT INTO schema_migrations(version, name, applied_at) VALUES
			(1, 'core schema', 1),
			(2, 'messages display_text column', 2);

		INSERT INTO messages(
			chat_jid, chat_name, msg_id, sender_jid, sender_name, ts, from_me, text, display_text, media_type
		) VALUES (
			'123@s.whatsapp.net', 'Alice', 'mid', '123@s.whatsapp.net', 'Alice', 1704067200, 0, '', '(message)', ''
		);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	opened, err := Open(path)
	if err != nil {
		t.Fatalf("Open migrated db: %v", err)
	}
	defer opened.Close()

	cols, err := tableColumns(opened.sql, "messages")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if !cols["message_kind"] || !cols["raw_summary"] {
		t.Fatalf("expected migrated columns to exist, got %#v", cols)
	}

	msg, err := opened.GetMessage("123@s.whatsapp.net", "mid")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.DisplayText != "(message)" {
		t.Fatalf("expected existing display text preserved, got %q", msg.DisplayText)
	}
	if msg.MessageKind != "legacy_unknown" {
		t.Fatalf("expected legacy placeholder message kind, got %q", msg.MessageKind)
	}
	if msg.RawSummary != "legacy_placeholder" {
		t.Fatalf("expected legacy placeholder raw summary, got %q", msg.RawSummary)
	}
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	return cols, rows.Err()
}
