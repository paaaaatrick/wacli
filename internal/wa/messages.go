package wa

import (
	"fmt"
	"slices"
	"strings"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Media struct {
	Type          string
	Caption       string
	Filename      string
	MimeType      string
	DirectPath    string
	MediaKey      []byte
	FileSHA256    []byte
	FileEncSHA256 []byte
	FileLength    uint64
}

type ParsedMessage struct {
	Chat           types.JID
	ID             string
	SenderJID      string
	Timestamp      time.Time
	FromMe         bool
	Text           string
	Media          *Media
	MessageKind    string
	RawSummary     string
	PushName       string
	ReplyToID      string
	ReplyToDisplay string
	ReactionToID   string
	ReactionEmoji  string
}

func DisplayText(pm ParsedMessage) string {
	if pm.Media != nil {
		return "Sent " + mediaLabel(pm.Media.Type)
	}
	if text := strings.TrimSpace(pm.Text); text != "" {
		return text
	}
	if text := KindDisplayText(pm.MessageKind); text != "" {
		return text
	}
	return ""
}

func ParseLiveMessage(evt *events.Message) ParsedMessage {
	msg := ParsedMessage{
		Chat:      evt.Info.Chat,
		ID:        evt.Info.ID,
		Timestamp: evt.Info.Timestamp,
		FromMe:    evt.Info.IsFromMe,
		PushName:  evt.Info.PushName,
	}
	if s := evt.Info.Sender.String(); s != "" {
		msg.SenderJID = s
	}

	extractWAProto(evt.Message, &msg)
	return msg
}

func ParseHistoryMessage(chatJID string, hist *waProto.WebMessageInfo) ParsedMessage {
	var chat types.JID
	if parsed, err := types.ParseJID(chatJID); err == nil {
		chat = parsed
	}

	pm := ParsedMessage{
		Chat:      chat,
		ID:        hist.GetKey().GetID(),
		Timestamp: time.Unix(int64(hist.GetMessageTimestamp()), 0).UTC(),
		FromMe:    hist.GetKey().GetFromMe(),
	}

	sender := strings.TrimSpace(hist.GetKey().GetParticipant())
	if sender == "" {
		sender = strings.TrimSpace(hist.GetKey().GetRemoteJID())
	}
	pm.SenderJID = sender

	if hist.GetMessage() != nil {
		extractWAProto(hist.GetMessage(), &pm)
	}
	return pm
}

func extractWAProto(m *waProto.Message, pm *ParsedMessage) {
	if m == nil || pm == nil {
		return
	}
	m = unwrapMessage(m)
	if m == nil {
		return
	}

	if reaction := m.GetReactionMessage(); reaction != nil {
		pm.MessageKind = "reaction"
		pm.ReactionEmoji = reaction.GetText()
		if key := reaction.GetKey(); key != nil {
			pm.ReactionToID = key.GetID()
		}
	} else if encReaction := m.GetEncReactionMessage(); encReaction != nil {
		pm.MessageKind = "reaction"
		if key := encReaction.GetTargetMessageKey(); key != nil {
			pm.ReactionToID = key.GetID()
		}
	}

	switch {
	case m.GetConversation() != "":
		pm.Text = m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		pm.Text = m.GetExtendedTextMessage().GetText()
	}

	if img := m.GetImageMessage(); img != nil {
		pm.MessageKind = "image"
		if pm.Text == "" {
			pm.Text = img.GetCaption()
		}
		pm.Media = &Media{
			Type:          "image",
			Caption:       img.GetCaption(),
			MimeType:      img.GetMimetype(),
			DirectPath:    img.GetDirectPath(),
			MediaKey:      clone(img.GetMediaKey()),
			FileSHA256:    clone(img.GetFileSHA256()),
			FileEncSHA256: clone(img.GetFileEncSHA256()),
			FileLength:    img.GetFileLength(),
		}
	}

	if vid := m.GetVideoMessage(); vid != nil {
		if pm.Text == "" {
			pm.Text = vid.GetCaption()
		}
		mediaType := "video"
		if vid.GetGifPlayback() {
			mediaType = "gif"
		}
		pm.MessageKind = mediaType
		pm.Media = &Media{
			Type:          mediaType,
			Caption:       vid.GetCaption(),
			MimeType:      vid.GetMimetype(),
			DirectPath:    vid.GetDirectPath(),
			MediaKey:      clone(vid.GetMediaKey()),
			FileSHA256:    clone(vid.GetFileSHA256()),
			FileEncSHA256: clone(vid.GetFileEncSHA256()),
			FileLength:    vid.GetFileLength(),
		}
	}

	if aud := m.GetAudioMessage(); aud != nil {
		pm.MessageKind = "audio"
		if pm.Text == "" {
			pm.Text = "[Audio]"
		}
		pm.Media = &Media{
			Type:          "audio",
			Caption:       pm.Text,
			MimeType:      aud.GetMimetype(),
			DirectPath:    aud.GetDirectPath(),
			MediaKey:      clone(aud.GetMediaKey()),
			FileSHA256:    clone(aud.GetFileSHA256()),
			FileEncSHA256: clone(aud.GetFileEncSHA256()),
			FileLength:    aud.GetFileLength(),
		}
	}

	if doc := m.GetDocumentMessage(); doc != nil {
		pm.MessageKind = "document"
		if pm.Text == "" {
			pm.Text = doc.GetCaption()
		}
		pm.Media = &Media{
			Type:          "document",
			Caption:       doc.GetCaption(),
			Filename:      doc.GetFileName(),
			MimeType:      doc.GetMimetype(),
			DirectPath:    doc.GetDirectPath(),
			MediaKey:      clone(doc.GetMediaKey()),
			FileSHA256:    clone(doc.GetFileSHA256()),
			FileEncSHA256: clone(doc.GetFileEncSHA256()),
			FileLength:    doc.GetFileLength(),
		}
	}

	if sticker := m.GetStickerMessage(); sticker != nil {
		pm.MessageKind = "sticker"
		pm.Media = &Media{
			Type:          "sticker",
			MimeType:      sticker.GetMimetype(),
			DirectPath:    sticker.GetDirectPath(),
			MediaKey:      clone(sticker.GetMediaKey()),
			FileSHA256:    clone(sticker.GetFileSHA256()),
			FileEncSHA256: clone(sticker.GetFileEncSHA256()),
			FileLength:    sticker.GetFileLength(),
		}
	}

	if loc := m.GetLocationMessage(); loc != nil {
		pm.MessageKind = "location"
		pm.Media = &Media{Type: "location"}
	}

	if liveLoc := m.GetLiveLocationMessage(); liveLoc != nil {
		pm.MessageKind = "live_location"
		setTextIfEmpty(pm, liveLoc.GetCaption())
	}

	if contact := m.GetContactMessage(); contact != nil {
		pm.MessageKind = "contact"
		pm.Media = &Media{Type: "contact"}
	}

	if contacts := m.GetContactsArrayMessage(); contacts != nil {
		pm.MessageKind = "contacts"
		pm.Media = &Media{Type: "contacts"}
	}

	if buttons := m.GetButtonsMessage(); buttons != nil {
		pm.MessageKind = "buttons"
		setTextIfEmpty(pm, buttonsMessageSummary(buttons))
	}

	if response := m.GetButtonsResponseMessage(); response != nil {
		pm.MessageKind = "buttons_response"
		setTextIfEmpty(pm, selectedValueSummary("Selected button", response.GetSelectedDisplayText()))
	}

	if list := m.GetListMessage(); list != nil {
		pm.MessageKind = "list"
		setTextIfEmpty(pm, listMessageSummary(list))
	}

	if response := m.GetListResponseMessage(); response != nil {
		pm.MessageKind = "list_response"
		setTextIfEmpty(pm, selectedValueSummary("Selected list item", firstNonEmpty(
			response.GetTitle(),
			response.GetDescription(),
		)))
	}

	if interactive := m.GetInteractiveMessage(); interactive != nil {
		pm.MessageKind = "interactive"
		setTextIfEmpty(pm, interactiveMessageSummary(interactive))
	}

	if response := m.GetInteractiveResponseMessage(); response != nil {
		pm.MessageKind = "interactive_response"
		setTextIfEmpty(pm, selectedValueSummary("Selected interactive response", firstNonEmpty(
			response.GetBody().GetText(),
			response.GetNativeFlowResponseMessage().GetName(),
		)))
	}

	if poll := pollCreationMessageFor(m); poll != nil {
		pm.MessageKind = "poll"
		setTextIfEmpty(pm, pollCreationSummary(poll))
	}

	if m.GetPollUpdateMessage() != nil {
		pm.MessageKind = "poll_update"
	}

	if m.GetRequestPhoneNumberMessage() != nil {
		pm.MessageKind = "request_phone_number"
	}

	if template := m.GetTemplateMessage(); template != nil {
		pm.MessageKind = "template"
		setTextIfEmpty(pm, templateMessageSummary(template))
	}

	if reply := m.GetTemplateButtonReplyMessage(); reply != nil {
		pm.MessageKind = "template_button_reply"
		setTextIfEmpty(pm, selectedValueSummary("Selected button", reply.GetSelectedDisplayText()))
	}

	if product := m.GetProductMessage(); product != nil {
		pm.MessageKind = "product"
		setTextIfEmpty(pm, productMessageSummary(product))
	}

	if order := m.GetOrderMessage(); order != nil {
		pm.MessageKind = "order"
		setTextIfEmpty(pm, orderMessageSummary(order))
	}

	if event := m.GetEventMessage(); event != nil {
		pm.MessageKind = "event"
		setTextIfEmpty(pm, eventMessageSummary(event))
	}

	if protocol := m.GetProtocolMessage(); protocol != nil {
		pm.MessageKind = "protocol_message"
		setTextIfEmpty(pm, protocolMessageSummary(protocol))
	}

	if ctx := contextInfoForMessage(m); ctx != nil {
		if id := strings.TrimSpace(ctx.GetStanzaID()); id != "" {
			pm.ReplyToID = id
		}
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			pm.ReplyToDisplay = strings.TrimSpace(displayTextForProto(quoted))
		}
	}

	if pm.MessageKind == "" && strings.TrimSpace(pm.Text) != "" {
		pm.MessageKind = "text"
	}
	if pm.MessageKind == "" {
		pm.MessageKind = primaryFieldName(m)
	}
	if shouldCaptureRawSummary(pm.MessageKind) {
		pm.RawSummary = summarizeProtoFields(m)
	}
}

func clone(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func mediaLabel(mediaType string) string {
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	switch mt {
	case "gif":
		return "gif"
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	case "sticker":
		return "sticker"
	case "document":
		return "document"
	case "location":
		return "location"
	case "contact":
		return "contact"
	case "contacts":
		return "contacts"
	case "":
		return "message"
	default:
		return mt
	}
}

func unwrapMessage(m *waProto.Message) *waProto.Message {
	for m != nil {
		switch {
		case m.GetDeviceSentMessage().GetMessage() != nil:
			m = m.GetDeviceSentMessage().GetMessage()
		case m.GetBotInvokeMessage().GetMessage() != nil:
			m = m.GetBotInvokeMessage().GetMessage()
		case m.GetEphemeralMessage().GetMessage() != nil:
			m = m.GetEphemeralMessage().GetMessage()
		case m.GetViewOnceMessage().GetMessage() != nil:
			m = m.GetViewOnceMessage().GetMessage()
		case m.GetViewOnceMessageV2().GetMessage() != nil:
			m = m.GetViewOnceMessageV2().GetMessage()
		case m.GetViewOnceMessageV2Extension().GetMessage() != nil:
			m = m.GetViewOnceMessageV2Extension().GetMessage()
		case m.GetLottieStickerMessage().GetMessage() != nil:
			m = m.GetLottieStickerMessage().GetMessage()
		case m.GetDocumentWithCaptionMessage().GetMessage() != nil:
			m = m.GetDocumentWithCaptionMessage().GetMessage()
		case m.GetEditedMessage().GetMessage() != nil:
			m = m.GetEditedMessage().GetMessage()
		case m.GetGroupMentionedMessage().GetMessage() != nil:
			m = m.GetGroupMentionedMessage().GetMessage()
		case m.GetPollCreationMessageV4().GetMessage() != nil:
			m = m.GetPollCreationMessageV4().GetMessage()
		case m.GetPollCreationMessageV6().GetMessage() != nil:
			m = m.GetPollCreationMessageV6().GetMessage()
		default:
			return m
		}
	}
	return nil
}

func contextInfoForMessage(m *waProto.Message) *waProto.ContextInfo {
	m = unwrapMessage(m)
	if m == nil {
		return nil
	}
	if ext := m.GetExtendedTextMessage(); ext != nil {
		return ext.GetContextInfo()
	}
	if img := m.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	if vid := m.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo()
	}
	if aud := m.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo()
	}
	if doc := m.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo()
	}
	if sticker := m.GetStickerMessage(); sticker != nil {
		return sticker.GetContextInfo()
	}
	if loc := m.GetLocationMessage(); loc != nil {
		return loc.GetContextInfo()
	}
	if contact := m.GetContactMessage(); contact != nil {
		return contact.GetContextInfo()
	}
	if contacts := m.GetContactsArrayMessage(); contacts != nil {
		return contacts.GetContextInfo()
	}
	if liveLoc := m.GetLiveLocationMessage(); liveLoc != nil {
		return liveLoc.GetContextInfo()
	}
	if buttons := m.GetButtonsMessage(); buttons != nil {
		return buttons.GetContextInfo()
	}
	if response := m.GetButtonsResponseMessage(); response != nil {
		return response.GetContextInfo()
	}
	if list := m.GetListMessage(); list != nil {
		return list.GetContextInfo()
	}
	if response := m.GetListResponseMessage(); response != nil {
		return response.GetContextInfo()
	}
	if interactive := m.GetInteractiveMessage(); interactive != nil {
		return interactive.GetContextInfo()
	}
	if response := m.GetInteractiveResponseMessage(); response != nil {
		return response.GetContextInfo()
	}
	if poll := pollCreationMessageFor(m); poll != nil {
		return poll.GetContextInfo()
	}
	if m.GetRequestPhoneNumberMessage() != nil {
		return m.GetRequestPhoneNumberMessage().GetContextInfo()
	}
	if template := m.GetTemplateMessage(); template != nil {
		return template.GetContextInfo()
	}
	if reply := m.GetTemplateButtonReplyMessage(); reply != nil {
		return reply.GetContextInfo()
	}
	if product := m.GetProductMessage(); product != nil {
		return product.GetContextInfo()
	}
	if order := m.GetOrderMessage(); order != nil {
		return order.GetContextInfo()
	}
	if event := m.GetEventMessage(); event != nil {
		return event.GetContextInfo()
	}
	return nil
}

func displayTextForProto(m *waProto.Message) string {
	if m == nil {
		return ""
	}
	var pm ParsedMessage
	extractWAProto(m, &pm)
	return DisplayText(pm)
}

func KindDisplayText(kind string) string {
	kind = strings.TrimSpace(kind)
	switch kind {
	case "", "text", "image", "video", "gif", "audio", "document", "sticker", "reaction":
		return ""
	case "location", "contact", "contacts":
		return "Sent " + strings.ReplaceAll(kind, "_", " ")
	case "live_location":
		return "Shared live location"
	case "buttons":
		return "Sent interactive buttons"
	case "buttons_response":
		return "Selected button"
	case "list":
		return "Sent list"
	case "list_response":
		return "Selected list item"
	case "interactive":
		return "Sent interactive message"
	case "interactive_response":
		return "Selected interactive response"
	case "poll":
		return "Created poll"
	case "poll_update":
		return "Updated poll vote"
	case "request_phone_number":
		return "Requested phone number"
	case "template":
		return "Sent template message"
	case "template_button_reply":
		return "Selected button"
	case "product":
		return "Shared product"
	case "order":
		return "Shared order"
	case "event":
		return "Shared event"
	default:
		return fmt.Sprintf("Unsupported message (%s)", kind)
	}
}

func shouldCaptureRawSummary(kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	return !slices.Contains([]string{
		"text",
		"image",
		"video",
		"gif",
		"audio",
		"document",
		"sticker",
		"location",
		"live_location",
		"contact",
		"contacts",
		"buttons",
		"buttons_response",
		"list",
		"list_response",
		"interactive",
		"interactive_response",
		"poll",
		"poll_update",
		"request_phone_number",
		"template",
		"template_button_reply",
		"product",
		"order",
		"event",
		"reaction",
	}, kind)
}

func primaryFieldName(m *waProto.Message) string {
	m = unwrapMessage(m)
	if m == nil {
		return ""
	}

	mr := m.ProtoReflect()
	fields := mr.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if mr.Has(fd) {
			return string(fd.Name())
		}
	}
	return ""
}

func summarizeProtoFields(m *waProto.Message) string {
	m = unwrapMessage(m)
	if m == nil {
		return ""
	}

	names := make([]string, 0, 4)
	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if len(names) >= 4 {
			return false
		}
		names = append(names, string(fd.Name()))
		return true
	})
	if len(names) == 0 {
		return ""
	}
	return "fields=" + strings.Join(names, ",")
}

func setTextIfEmpty(pm *ParsedMessage, text string) {
	if pm == nil || strings.TrimSpace(pm.Text) != "" {
		return
	}
	pm.Text = strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func joinSummary(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}
	switch len(nonEmpty) {
	case 0:
		return ""
	case 1:
		return nonEmpty[0]
	default:
		return nonEmpty[0] + ": " + strings.Join(nonEmpty[1:], " - ")
	}
}

func selectedValueSummary(prefix, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return prefix + ": " + value
}

func joinedNonEmpty(values []string, sep string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, sep)
}

func buttonLabels(buttons []*waProto.ButtonsMessage_Button) []string {
	labels := make([]string, 0, min(len(buttons), 3))
	for _, button := range buttons {
		if button == nil {
			continue
		}
		label := firstNonEmpty(
			button.GetButtonText().GetDisplayText(),
			button.GetNativeFlowInfo().GetName(),
		)
		if label == "" {
			continue
		}
		labels = append(labels, label)
		if len(labels) >= 3 {
			break
		}
	}
	return labels
}

func hydratedTemplateButtonLabels(buttons []*waProto.HydratedTemplateButton) []string {
	labels := make([]string, 0, min(len(buttons), 3))
	for _, button := range buttons {
		if button == nil {
			continue
		}
		label := firstNonEmpty(
			button.GetQuickReplyButton().GetDisplayText(),
			button.GetUrlButton().GetDisplayText(),
			button.GetCallButton().GetDisplayText(),
		)
		if label == "" {
			continue
		}
		labels = append(labels, label)
		if len(labels) >= 3 {
			break
		}
	}
	return labels
}

func templateButtonLabels(buttons []*waProto.TemplateButton) []string {
	labels := make([]string, 0, min(len(buttons), 3))
	for _, button := range buttons {
		if button == nil {
			continue
		}
		label := firstNonEmpty(
			highlyStructuredMessageText(button.GetQuickReplyButton().GetDisplayText()),
			highlyStructuredMessageText(button.GetUrlButton().GetDisplayText()),
			highlyStructuredMessageText(button.GetCallButton().GetDisplayText()),
		)
		if label == "" {
			continue
		}
		labels = append(labels, label)
		if len(labels) >= 3 {
			break
		}
	}
	return labels
}

func nativeFlowButtonNames(buttons []*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton) []string {
	names := make([]string, 0, min(len(buttons), 3))
	for _, button := range buttons {
		if button == nil {
			continue
		}
		if name := strings.TrimSpace(button.GetName()); name != "" {
			names = append(names, name)
		}
		if len(names) >= 3 {
			break
		}
	}
	return names
}

func buttonsMessageSummary(buttons *waProto.ButtonsMessage) string {
	if buttons == nil {
		return ""
	}
	return firstNonEmpty(
		buttons.GetContentText(),
		buttons.GetText(),
		joinSummary("Buttons", strings.Join(buttonLabels(buttons.GetButtons()), ", ")),
		buttons.GetFooterText(),
	)
}

func listMessageSummary(list *waProto.ListMessage) string {
	if list == nil {
		return ""
	}
	return firstNonEmpty(
		joinSummary(list.GetTitle(), list.GetDescription()),
		list.GetTitle(),
		list.GetDescription(),
		joinSummary("List", list.GetButtonText()),
		list.GetFooterText(),
	)
}

func interactiveMessageSummary(interactive *waProto.InteractiveMessage) string {
	if interactive == nil {
		return ""
	}
	return firstNonEmpty(
		interactive.GetBody().GetText(),
		joinSummary(interactive.GetHeader().GetTitle(), interactive.GetHeader().GetSubtitle()),
		joinSummary("Interactive", strings.Join(nativeFlowButtonNames(interactive.GetNativeFlowMessage().GetButtons()), ", ")),
		interactive.GetFooter().GetText(),
	)
}

func templateMessageSummary(template *waProto.TemplateMessage) string {
	if template == nil {
		return ""
	}

	if summary := interactiveMessageSummary(template.GetInteractiveMessageTemplate()); summary != "" {
		return summary
	}

	if hydrated := firstNonNilTemplateHydrated(template); hydrated != nil {
		return firstNonEmpty(
			joinSummary(
				firstNonEmpty(hydrated.GetHydratedTitleText(), templateTitleMediaSummary(
					hydrated.GetDocumentMessage(),
					hydrated.GetImageMessage(),
					hydrated.GetVideoMessage(),
					hydrated.GetLocationMessage(),
				)),
				hydrated.GetHydratedContentText(),
			),
			hydrated.GetHydratedContentText(),
			joinSummary("Template", strings.Join(hydratedTemplateButtonLabels(hydrated.GetHydratedButtons()), ", ")),
			hydrated.GetHydratedFooterText(),
		)
	}

	if fourRow := template.GetFourRowTemplate(); fourRow != nil {
		return firstNonEmpty(
			joinSummary(
				firstNonEmpty(
					highlyStructuredMessageText(fourRow.GetHighlyStructuredMessage()),
					templateTitleMediaSummary(
						fourRow.GetDocumentMessage(),
						fourRow.GetImageMessage(),
						fourRow.GetVideoMessage(),
						fourRow.GetLocationMessage(),
					),
				),
				highlyStructuredMessageText(fourRow.GetContent()),
			),
			highlyStructuredMessageText(fourRow.GetContent()),
			joinSummary("Template", strings.Join(templateButtonLabels(fourRow.GetButtons()), ", ")),
			highlyStructuredMessageText(fourRow.GetFooter()),
		)
	}

	if template.GetTemplateID() != "" {
		return "Template: " + template.GetTemplateID()
	}

	return ""
}

func firstNonNilTemplateHydrated(template *waProto.TemplateMessage) *waProto.TemplateMessage_HydratedFourRowTemplate {
	if template == nil {
		return nil
	}
	if hydrated := template.GetHydratedFourRowTemplate(); hydrated != nil {
		return hydrated
	}
	if hydrated := template.GetHydratedTemplate(); hydrated != nil {
		return hydrated
	}
	return nil
}

func templateTitleMediaSummary(document *waProto.DocumentMessage, image *waProto.ImageMessage, video *waProto.VideoMessage, location *waProto.LocationMessage) string {
	switch {
	case document != nil:
		return "Sent document"
	case image != nil:
		return "Sent image"
	case video != nil:
		return "Sent video"
	case location != nil:
		return "Sent location"
	default:
		return ""
	}
}

func highlyStructuredMessageText(hsm *waProto.HighlyStructuredMessage) string {
	if hsm == nil {
		return ""
	}
	if nested := hsm.GetHydratedHsm(); nested != nil {
		if summary := templateMessageSummary(nested); summary != "" {
			return summary
		}
	}
	if params := joinedNonEmpty(hsm.GetParams(), " "); params != "" {
		return params
	}
	return ""
}

func pollCreationMessageFor(m *waProto.Message) *waProto.PollCreationMessage {
	if m == nil {
		return nil
	}
	switch {
	case m.GetPollCreationMessage() != nil:
		return m.GetPollCreationMessage()
	case m.GetPollCreationMessageV2() != nil:
		return m.GetPollCreationMessageV2()
	case m.GetPollCreationMessageV3() != nil:
		return m.GetPollCreationMessageV3()
	case m.GetPollCreationMessageV5() != nil:
		return m.GetPollCreationMessageV5()
	default:
		return nil
	}
}

func pollCreationSummary(poll *waProto.PollCreationMessage) string {
	if poll == nil {
		return ""
	}
	name := strings.TrimSpace(poll.GetName())
	if name == "" {
		return ""
	}
	return "Poll: " + name
}

func productMessageSummary(product *waProto.ProductMessage) string {
	if product == nil {
		return ""
	}
	return firstNonEmpty(
		joinSummary("Product", firstNonEmpty(
			product.GetProduct().GetTitle(),
			product.GetCatalog().GetTitle(),
		), firstNonEmpty(
			product.GetBody(),
			product.GetProduct().GetDescription(),
			product.GetCatalog().GetDescription(),
		)),
		product.GetBody(),
		product.GetProduct().GetTitle(),
		product.GetCatalog().GetTitle(),
		product.GetProduct().GetDescription(),
	)
}

func orderMessageSummary(order *waProto.OrderMessage) string {
	if order == nil {
		return ""
	}
	return firstNonEmpty(
		joinSummary("Order", order.GetOrderTitle(), order.GetMessage(), itemCountSummary(order.GetItemCount())),
		order.GetMessage(),
		order.GetOrderTitle(),
		itemCountSummary(order.GetItemCount()),
	)
}

func itemCountSummary(count int32) string {
	if count <= 0 {
		return ""
	}
	if count == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", count)
}

func eventMessageSummary(event *waProto.EventMessage) string {
	if event == nil {
		return ""
	}
	name := strings.TrimSpace(event.GetName())
	if name == "" {
		if event.GetIsCanceled() {
			return "Canceled event"
		}
		return ""
	}
	if event.GetIsCanceled() {
		return "Canceled event: " + name
	}
	return "Event: " + name
}

func protocolMessageSummary(protocol *waProto.ProtocolMessage) string {
	if protocol == nil {
		return ""
	}
	switch protocol.GetType() {
	case waProto.ProtocolMessage_REVOKE:
		return "Deleted message"
	case waProto.ProtocolMessage_EPHEMERAL_SETTING:
		return "Updated disappearing messages"
	case waProto.ProtocolMessage_SHARE_PHONE_NUMBER:
		return "Shared phone number"
	case waProto.ProtocolMessage_MESSAGE_EDIT:
		if edited := strings.TrimSpace(displayTextForProto(protocol.GetEditedMessage())); edited != "" {
			return "Edited message: " + edited
		}
		return "Edited message"
	default:
		return ""
	}
}
