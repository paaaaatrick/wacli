package wa

import (
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestParseHistoryMessageTextAndSender(t *testing.T) {
	h := &waProto.WebMessageInfo{
		Key: &waProto.MessageKey{
			ID:          proto.String("msgid"),
			FromMe:      proto.Bool(false),
			Participant: proto.String("sender@s.whatsapp.net"),
		},
		MessageTimestamp: proto.Uint64(uint64(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix())),
		Message:          &waProto.Message{Conversation: proto.String("hello")},
	}
	pm := ParseHistoryMessage("123@s.whatsapp.net", h)
	if pm.ID != "msgid" || pm.Text != "hello" {
		t.Fatalf("unexpected parsed msg: %+v", pm)
	}
	if pm.SenderJID != "sender@s.whatsapp.net" {
		t.Fatalf("unexpected sender: %q", pm.SenderJID)
	}
	if pm.Chat.String() != "123@s.whatsapp.net" {
		t.Fatalf("unexpected chat: %q", pm.Chat.String())
	}
}

func TestParseLiveMessageImageClonesBytes(t *testing.T) {
	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("sender@s.whatsapp.net")

	key := []byte{1, 2, 3}
	img := &waProto.ImageMessage{
		Caption:       proto.String("cap"),
		Mimetype:      proto.String("image/jpeg"),
		DirectPath:    proto.String("/direct"),
		MediaKey:      key,
		FileSHA256:    []byte{4},
		FileEncSHA256: []byte{5},
		FileLength:    proto.Uint64(10),
	}
	ev := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   sender,
				IsFromMe: false,
			},
			ID:        "mid",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PushName:  "Sender",
		},
		Message: &waProto.Message{ImageMessage: img},
	}

	pm := ParseLiveMessage(ev)
	if pm.ID != "mid" || pm.Media == nil || pm.Media.Type != "image" {
		t.Fatalf("unexpected parsed: %+v", pm)
	}
	if pm.Text != "cap" {
		t.Fatalf("expected text from caption, got %q", pm.Text)
	}

	// Ensure clone() was used (pm.Media.MediaKey should not alias key).
	key[0] = 9
	if pm.Media.MediaKey[0] == 9 {
		t.Fatalf("expected MediaKey to be cloned")
	}
}

func TestParseLiveMessageReaction(t *testing.T) {
	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("sender@s.whatsapp.net")

	ev := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   sender,
				IsFromMe: false,
			},
			ID:        "mid",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PushName:  "Sender",
		},
		Message: &waProto.Message{
			ReactionMessage: &waProto.ReactionMessage{
				Text: proto.String("👍"),
				Key:  &waProto.MessageKey{ID: proto.String("orig")},
			},
		},
	}

	pm := ParseLiveMessage(ev)
	if pm.ReactionEmoji != "👍" || pm.ReactionToID != "orig" {
		t.Fatalf("unexpected reaction parse: %+v", pm)
	}
}

func TestParseLiveMessageReply(t *testing.T) {
	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("sender@s.whatsapp.net")

	ev := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   sender,
				IsFromMe: false,
			},
			ID:        "mid",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PushName:  "Sender",
		},
		Message: &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String("reply text"),
				ContextInfo: &waProto.ContextInfo{
					StanzaID: proto.String("orig"),
					QuotedMessage: &waProto.Message{
						Conversation: proto.String("quoted"),
					},
				},
			},
		},
	}

	pm := ParseLiveMessage(ev)
	if pm.ReplyToID != "orig" {
		t.Fatalf("expected ReplyToID to be orig, got %q", pm.ReplyToID)
	}
	if pm.ReplyToDisplay != "quoted" {
		t.Fatalf("expected ReplyToDisplay to be quoted, got %q", pm.ReplyToDisplay)
	}
}

func TestParseLiveMessageLocationClassifiedAsKnownKind(t *testing.T) {
	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("sender@s.whatsapp.net")

	msg := &waProto.Message{}
	if !setEmptyMessageFieldByName(msg, "location_message") {
		t.Skip("location_message not present in current proto")
	}

	ev := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   sender,
				IsFromMe: false,
			},
			ID:        "mid",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PushName:  "Sender",
		},
		Message: msg,
	}

	pm := ParseLiveMessage(ev)
	if pm.MessageKind != "location" {
		t.Fatalf("expected MessageKind location, got %q", pm.MessageKind)
	}
	if pm.Media == nil || pm.Media.Type != "location" {
		t.Fatalf("expected location media, got %+v", pm.Media)
	}
	if pm.RawSummary != "" {
		t.Fatalf("expected location not to capture raw summary, got %q", pm.RawSummary)
	}
}

func TestParseLiveMessageUnknownKindPreservesRawSummary(t *testing.T) {
	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("sender@s.whatsapp.net")

	msg := &waProto.Message{}
	fieldName, ok := setFirstAvailableMessageField(msg,
		"protocol_message",
		"poll_creation_message",
		"interactive_message",
		"live_location_message",
	)
	if !ok {
		t.Skip("no suitable opaque message field present in current proto")
	}

	ev := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   sender,
				IsFromMe: false,
			},
			ID:        "mid",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PushName:  "Sender",
		},
		Message: msg,
	}

	pm := ParseLiveMessage(ev)
	if pm.MessageKind != fieldName {
		t.Fatalf("expected MessageKind %q, got %q", fieldName, pm.MessageKind)
	}
	if pm.RawSummary == "" {
		t.Fatalf("expected RawSummary for opaque message")
	}
	if got := KindDisplayText(pm.MessageKind); got == "" {
		t.Fatalf("expected fallback display text for opaque kind %q", pm.MessageKind)
	}
}

func TestParseHistoryMessageUnwrapsStructuredPoll(t *testing.T) {
	h := &waProto.WebMessageInfo{
		Key: &waProto.MessageKey{
			ID:          proto.String("poll-msg"),
			FromMe:      proto.Bool(false),
			Participant: proto.String("sender@s.whatsapp.net"),
		},
		MessageTimestamp: proto.Uint64(uint64(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC).Unix())),
		Message: &waProto.Message{
			DeviceSentMessage: &waProto.DeviceSentMessage{
				Message: &waProto.Message{
					EphemeralMessage: &waProto.FutureProofMessage{
						Message: &waProto.Message{
							PollCreationMessage: &waProto.PollCreationMessage{
								Name: proto.String("Lunch?"),
							},
						},
					},
				},
			},
		},
	}

	pm := ParseHistoryMessage("123@s.whatsapp.net", h)
	if pm.MessageKind != "poll" {
		t.Fatalf("expected MessageKind poll, got %q", pm.MessageKind)
	}
	if pm.Text != "Poll: Lunch?" {
		t.Fatalf("expected poll summary, got %q", pm.Text)
	}
	if pm.RawSummary != "" {
		t.Fatalf("expected known poll not to capture raw summary, got %q", pm.RawSummary)
	}
}

func TestParseLiveMessageStructuredKinds(t *testing.T) {
	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("sender@s.whatsapp.net")

	cases := []struct {
		name            string
		message         *waProto.Message
		wantKind        string
		wantText        string
		wantRawSummary  bool
		wantDisplayText string
	}{
		{
			name: "live location",
			message: &waProto.Message{
				LiveLocationMessage: &waProto.LiveLocationMessage{Caption: proto.String("On my way")},
			},
			wantKind:        "live_location",
			wantText:        "On my way",
			wantDisplayText: "On my way",
		},
		{
			name: "buttons message",
			message: &waProto.Message{
				ButtonsMessage: &waProto.ButtonsMessage{
					ContentText: proto.String("Choose one"),
					Buttons: []*waProto.ButtonsMessage_Button{{
						ButtonText: &waProto.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Yes")},
					}},
				},
			},
			wantKind:        "buttons",
			wantText:        "Choose one",
			wantDisplayText: "Choose one",
		},
		{
			name: "buttons response",
			message: &waProto.Message{
				ButtonsResponseMessage: &waProto.ButtonsResponseMessage{
					Response: &waProto.ButtonsResponseMessage_SelectedDisplayText{SelectedDisplayText: "Yes"},
				},
			},
			wantKind:        "buttons_response",
			wantText:        "Selected button: Yes",
			wantDisplayText: "Selected button: Yes",
		},
		{
			name: "interactive response",
			message: &waProto.Message{
				InteractiveResponseMessage: &waProto.InteractiveResponseMessage{
					Body: &waProto.InteractiveResponseMessage_Body{Text: proto.String("Continue")},
				},
			},
			wantKind:        "interactive_response",
			wantText:        "Selected interactive response: Continue",
			wantDisplayText: "Selected interactive response: Continue",
		},
		{
			name: "request phone number",
			message: &waProto.Message{
				RequestPhoneNumberMessage: &waProto.RequestPhoneNumberMessage{},
			},
			wantKind:        "request_phone_number",
			wantDisplayText: "Requested phone number",
		},
		{
			name: "template message",
			message: &waProto.Message{
				TemplateMessage: &waProto.TemplateMessage{
					Format: &waProto.TemplateMessage_HydratedFourRowTemplate_{
						HydratedFourRowTemplate: &waProto.TemplateMessage_HydratedFourRowTemplate{
							Title:               &waProto.TemplateMessage_HydratedFourRowTemplate_HydratedTitleText{HydratedTitleText: "Verification"},
							HydratedContentText: proto.String("Use code 1234"),
							HydratedButtons: []*waProto.HydratedTemplateButton{{
								HydratedButton: &waProto.HydratedTemplateButton_QuickReplyButton{
									QuickReplyButton: &waProto.HydratedTemplateButton_HydratedQuickReplyButton{DisplayText: proto.String("Copy code")},
								},
							}},
						},
					},
				},
			},
			wantKind:        "template",
			wantText:        "Verification: Use code 1234",
			wantDisplayText: "Verification: Use code 1234",
		},
		{
			name: "template button reply",
			message: &waProto.Message{
				TemplateButtonReplyMessage: &waProto.TemplateButtonReplyMessage{
					SelectedDisplayText: proto.String("Call me"),
				},
			},
			wantKind:        "template_button_reply",
			wantText:        "Selected button: Call me",
			wantDisplayText: "Selected button: Call me",
		},
		{
			name: "product message",
			message: &waProto.Message{
				ProductMessage: &waProto.ProductMessage{
					Product: &waProto.ProductMessage_ProductSnapshot{
						Title:       proto.String("Trail Shoes"),
						Description: proto.String("Lightweight runner"),
					},
					Body: proto.String("Featured item"),
				},
			},
			wantKind:        "product",
			wantText:        "Product: Trail Shoes - Featured item",
			wantDisplayText: "Product: Trail Shoes - Featured item",
		},
		{
			name: "order message",
			message: &waProto.Message{
				OrderMessage: &waProto.OrderMessage{
					OrderTitle: proto.String("Weekend order"),
					Message:    proto.String("Ready for pickup"),
					ItemCount:  proto.Int32(2),
				},
			},
			wantKind:        "order",
			wantText:        "Order: Weekend order - Ready for pickup - 2 items",
			wantDisplayText: "Order: Weekend order - Ready for pickup - 2 items",
		},
		{
			name: "event message",
			message: &waProto.Message{
				EventMessage: &waProto.EventMessage{
					Name:       proto.String("Demo"),
					IsCanceled: proto.Bool(true),
				},
			},
			wantKind:        "event",
			wantText:        "Canceled event: Demo",
			wantDisplayText: "Canceled event: Demo",
		},
		{
			name: "protocol message",
			message: &waProto.Message{
				ProtocolMessage: &waProto.ProtocolMessage{
					Type: waProto.ProtocolMessage_REVOKE.Enum(),
				},
			},
			wantKind:        "protocol_message",
			wantText:        "Deleted message",
			wantRawSummary:  true,
			wantDisplayText: "Deleted message",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &events.Message{
				Info: types.MessageInfo{
					MessageSource: types.MessageSource{
						Chat:     chat,
						Sender:   sender,
						IsFromMe: false,
					},
					ID:        "mid",
					Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					PushName:  "Sender",
				},
				Message: tc.message,
			}

			pm := ParseLiveMessage(ev)
			if pm.MessageKind != tc.wantKind {
				t.Fatalf("expected MessageKind %q, got %q", tc.wantKind, pm.MessageKind)
			}
			if pm.Text != tc.wantText {
				t.Fatalf("expected Text %q, got %q", tc.wantText, pm.Text)
			}
			if got := DisplayText(pm); got != tc.wantDisplayText {
				t.Fatalf("expected display text %q, got %q", tc.wantDisplayText, got)
			}
			if tc.wantRawSummary && pm.RawSummary == "" {
				t.Fatalf("expected RawSummary for %s", tc.name)
			}
			if !tc.wantRawSummary && pm.RawSummary != "" {
				t.Fatalf("expected no RawSummary for %s, got %q", tc.name, pm.RawSummary)
			}
		})
	}
}

func TestParseLiveMessageReplyToStructuredQuotedMessage(t *testing.T) {
	chat, _ := types.ParseJID("123@s.whatsapp.net")
	sender, _ := types.ParseJID("sender@s.whatsapp.net")

	ev := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   sender,
				IsFromMe: false,
			},
			ID:        "mid",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PushName:  "Sender",
		},
		Message: &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String("reply text"),
				ContextInfo: &waProto.ContextInfo{
					StanzaID: proto.String("orig"),
					QuotedMessage: &waProto.Message{
						TemplateMessage: &waProto.TemplateMessage{
							Format: &waProto.TemplateMessage_HydratedFourRowTemplate_{
								HydratedFourRowTemplate: &waProto.TemplateMessage_HydratedFourRowTemplate{
									Title:               &waProto.TemplateMessage_HydratedFourRowTemplate_HydratedTitleText{HydratedTitleText: "Verification"},
									HydratedContentText: proto.String("Use code 1234"),
								},
							},
						},
					},
				},
			},
		},
	}

	pm := ParseLiveMessage(ev)
	if pm.ReplyToDisplay != "Verification: Use code 1234" {
		t.Fatalf("expected quoted structured display text, got %q", pm.ReplyToDisplay)
	}
}

func setFirstAvailableMessageField(msg *waProto.Message, candidates ...string) (string, bool) {
	for _, candidate := range candidates {
		if setEmptyMessageFieldByName(msg, candidate) {
			return candidate, true
		}
	}
	return "", false
}

func setEmptyMessageFieldByName(msg *waProto.Message, fieldName string) bool {
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
