//go:build sqlite_fts5

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSearchMessagesUsesFTSWhenEnabled(t *testing.T) {
	db := openTestDB(t)
	if !db.HasFTS() {
		t.Fatalf("expected HasFTS=true in sqlite_fts5 build")
	}

	chat := "123@s.whatsapp.net"
	if err := db.UpsertChat(chat, "dm", "Alice", time.Now()); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := db.UpsertMessage(UpsertMessageParams{
		ChatJID:    chat,
		ChatName:   "Alice",
		MsgID:      "m1",
		SenderJID:  chat,
		SenderName: "Alice",
		Timestamp:  time.Now(),
		FromMe:     false,
		Text:       "hello world",
	}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}

	ms, err := db.SearchMessages(SearchMessagesParams{Query: "hello", Limit: 10})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(ms) != 1 {
		t.Fatalf("expected 1 result, got %d", len(ms))
	}
	if ms[0].Snippet == "" {
		t.Fatalf("expected snippet for FTS search, got empty")
	}
}

func TestOpenRetainsFTSEnabledOnReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wacli.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !db.HasFTS() {
		t.Fatalf("expected HasFTS=true on initial open")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open reopen: %v", err)
	}
	defer reopened.Close()

	if !reopened.HasFTS() {
		t.Fatalf("expected HasFTS=true after reopen")
	}
}
