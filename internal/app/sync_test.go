package app

import (
	"context"
	"strings"
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestSyncStoresLiveAndHistoryMessages(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "123", Server: types.DefaultUserServer}
	f.contacts[chat.ToNonAD()] = types.ContactInfo{
		Found:     true,
		FullName:  "Alice",
		FirstName: "Alice",
		PushName:  "Alice",
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	live := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "m-live",
			Timestamp: base.Add(2 * time.Second),
			PushName:  "Alice",
		},
		Message: &waProto.Message{Conversation: proto.String("hello")},
	}

	histMsg := &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{
			RemoteJID: proto.String(chat.String()),
			FromMe:    proto.Bool(false),
			ID:        proto.String("m-hist"),
		},
		MessageTimestamp: proto.Uint64(uint64(base.Add(1 * time.Second).Unix())),
		Message:          &waProto.Message{Conversation: proto.String("older")},
	}
	history := &events.HistorySync{
		Data: &waHistorySync.HistorySync{
			SyncType: waHistorySync.HistorySync_FULL.Enum(),
			Conversations: []*waHistorySync.Conversation{{
				ID:       proto.String(chat.String()),
				Messages: []*waHistorySync.HistorySyncMsg{{Message: histMsg}},
			}},
		},
	}

	f.connectEvents = []interface{}{live, history}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	res, err := a.Sync(ctx, SyncOptions{
		Mode:    SyncModeFollow,
		AllowQR: false,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.MessagesStored != 2 {
		t.Fatalf("expected 2 MessagesStored, got %d", res.MessagesStored)
	}
	if n, err := a.db.CountMessages(); err != nil || n != 2 {
		t.Fatalf("expected 2 messages in DB, got %d (err=%v)", n, err)
	}
}

func TestSyncStoresDisplayText(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "123", Server: types.DefaultUserServer}
	f.contacts[chat.ToNonAD()] = types.ContactInfo{
		Found:     true,
		FullName:  "Alice",
		FirstName: "Alice",
		PushName:  "Alice",
	}

	base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	textMsg := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "m-text",
			Timestamp: base.Add(1 * time.Second),
			PushName:  "Alice",
		},
		Message: &waProto.Message{Conversation: proto.String("hello")},
	}

	imageMsg := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "m-image",
			Timestamp: base.Add(2 * time.Second),
			PushName:  "Alice",
		},
		Message: &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				Mimetype:      proto.String("image/jpeg"),
				DirectPath:    proto.String("/direct"),
				MediaKey:      []byte{1},
				FileSHA256:    []byte{2},
				FileEncSHA256: []byte{3},
				FileLength:    proto.Uint64(10),
			},
		},
	}

	replyMsg := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "m-reply",
			Timestamp: base.Add(3 * time.Second),
			PushName:  "Alice",
		},
		Message: &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String("reply text"),
				ContextInfo: &waProto.ContextInfo{
					StanzaID: proto.String("m-text"),
					QuotedMessage: &waProto.Message{
						Conversation: proto.String("quoted text"),
					},
				},
			},
		},
	}

	reactionMsg := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "m-react",
			Timestamp: base.Add(4 * time.Second),
			PushName:  "Alice",
		},
		Message: &waProto.Message{
			ReactionMessage: &waProto.ReactionMessage{
				Text: proto.String("👍"),
				Key:  &waProto.MessageKey{ID: proto.String("m-text")},
			},
		},
	}

	f.connectEvents = []interface{}{textMsg, imageMsg, replyMsg, reactionMsg}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	res, err := a.Sync(ctx, SyncOptions{
		Mode:    SyncModeFollow,
		AllowQR: false,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.MessagesStored != 4 {
		t.Fatalf("expected 4 MessagesStored, got %d", res.MessagesStored)
	}

	msg, err := a.db.GetMessage(chat.String(), "m-text")
	if err != nil {
		t.Fatalf("GetMessage text: %v", err)
	}
	if msg.DisplayText != "hello" {
		t.Fatalf("expected display text 'hello', got %q", msg.DisplayText)
	}

	msg, err = a.db.GetMessage(chat.String(), "m-image")
	if err != nil {
		t.Fatalf("GetMessage image: %v", err)
	}
	if msg.DisplayText != "Sent image" {
		t.Fatalf("expected display text 'Sent image', got %q", msg.DisplayText)
	}

	msg, err = a.db.GetMessage(chat.String(), "m-reply")
	if err != nil {
		t.Fatalf("GetMessage reply: %v", err)
	}
	if msg.DisplayText != "> quoted text\nreply text" {
		t.Fatalf("unexpected reply display text: %q", msg.DisplayText)
	}

	msg, err = a.db.GetMessage(chat.String(), "m-react")
	if err != nil {
		t.Fatalf("GetMessage react: %v", err)
	}
	if msg.DisplayText != "Reacted 👍 to hello" {
		t.Fatalf("unexpected reaction display text: %q", msg.DisplayText)
	}
}

func TestSyncStoresStructuredFallbackDisplayText(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "123", Server: types.DefaultUserServer}
	f.contacts[chat.ToNonAD()] = types.ContactInfo{
		Found:     true,
		FullName:  "Alice",
		FirstName: "Alice",
		PushName:  "Alice",
	}

	msg := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "m-request-phone",
			Timestamp: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
			PushName:  "Alice",
		},
		Message: &waProto.Message{
			RequestPhoneNumberMessage: &waProto.RequestPhoneNumberMessage{},
		},
	}

	f.connectEvents = []interface{}{msg}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	res, err := a.Sync(ctx, SyncOptions{
		Mode:    SyncModeFollow,
		AllowQR: false,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.MessagesStored != 1 {
		t.Fatalf("expected 1 MessagesStored, got %d", res.MessagesStored)
	}

	stored, err := a.db.GetMessage(chat.String(), "m-request-phone")
	if err != nil {
		t.Fatalf("GetMessage structured: %v", err)
	}
	if stored.DisplayText != "Requested phone number" {
		t.Fatalf("expected structured display text, got %q", stored.DisplayText)
	}
	if stored.MessageKind != "request_phone_number" {
		t.Fatalf("expected structured message kind, got %q", stored.MessageKind)
	}
}

func TestSyncStoresTemplateDisplayText(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "123", Server: types.DefaultUserServer}
	f.contacts[chat.ToNonAD()] = types.ContactInfo{
		Found:     true,
		FullName:  "Alice",
		FirstName: "Alice",
		PushName:  "Alice",
	}

	msg := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   chat,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "m-template",
			Timestamp: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
			PushName:  "Alice",
		},
		Message: &waProto.Message{
			TemplateMessage: &waProto.TemplateMessage{
				Format: &waProto.TemplateMessage_HydratedFourRowTemplate_{
					HydratedFourRowTemplate: &waProto.TemplateMessage_HydratedFourRowTemplate{
						Title:               &waProto.TemplateMessage_HydratedFourRowTemplate_HydratedTitleText{HydratedTitleText: "Verification"},
						HydratedContentText: proto.String("Use code 1234"),
					},
				},
			},
		},
	}

	f.connectEvents = []interface{}{msg}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	res, err := a.Sync(ctx, SyncOptions{
		Mode:    SyncModeFollow,
		AllowQR: false,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.MessagesStored != 1 {
		t.Fatalf("expected 1 MessagesStored, got %d", res.MessagesStored)
	}

	stored, err := a.db.GetMessage(chat.String(), "m-template")
	if err != nil {
		t.Fatalf("GetMessage template: %v", err)
	}
	if stored.DisplayText != "Verification: Use code 1234" {
		t.Fatalf("expected template display text, got %q", stored.DisplayText)
	}
	if stored.MessageKind != "template" {
		t.Fatalf("expected template message kind, got %q", stored.MessageKind)
	}
}

func TestSyncOnceIdleExit(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err := a.Sync(ctx, SyncOptions{
		Mode:     SyncModeOnce,
		AllowQR:  false,
		IdleExit: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("expected to exit quickly on idle, took %s", time.Since(start))
	}
}

func TestSyncStoresOpaqueMessageKindAndSummary(t *testing.T) {
	a := newTestApp(t)
	f := newFakeWA()
	a.wa = f

	chat := types.JID{User: "123", Server: types.DefaultUserServer}
	msg := &waProto.Message{}
	fieldName, ok := setOpaqueTestField(msg)
	if !ok {
		t.Skip("no suitable opaque message field present in current proto")
	}

	f.connectEvents = []interface{}{
		&events.Message{
			Info: types.MessageInfo{
				MessageSource: types.MessageSource{
					Chat:     chat,
					Sender:   chat,
					IsFromMe: false,
					IsGroup:  false,
				},
				ID:        "m-opaque",
				Timestamp: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				PushName:  "Alice",
			},
			Message: msg,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	if _, err := a.Sync(ctx, SyncOptions{Mode: SyncModeFollow, AllowQR: false}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	stored, err := a.db.GetMessage(chat.String(), "m-opaque")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.MessageKind != fieldName {
		t.Fatalf("expected MessageKind %q, got %q", fieldName, stored.MessageKind)
	}
	if stored.RawSummary == "" || !strings.Contains(stored.RawSummary, fieldName) {
		t.Fatalf("expected RawSummary to mention %q, got %q", fieldName, stored.RawSummary)
	}
	if stored.DisplayText != "Unsupported message ("+fieldName+")" {
		t.Fatalf("unexpected DisplayText: %q", stored.DisplayText)
	}
}

func setOpaqueTestField(msg *waProto.Message) (string, bool) {
	for _, fieldName := range []string{
		"protocol_message",
		"poll_creation_message",
		"interactive_message",
		"live_location_message",
	} {
		if setEmptyMessageField(msg, fieldName) {
			return fieldName, true
		}
	}
	return "", false
}

func setEmptyMessageField(msg *waProto.Message, fieldName string) bool {
	if msg == nil {
		return false
	}

	mr := msg.ProtoReflect()
	fd := mr.Descriptor().Fields().ByName(protoreflect.Name(fieldName))
	if fd == nil || fd.Kind() != protoreflect.MessageKind {
		return false
	}

	mr.Set(fd, mr.NewField(fd))
	return true
}
