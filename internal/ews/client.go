// Package ews reads and sends mail through Exchange Web Services on mail.spbstu.ru.
package ews

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
)

const Endpoint = "https://mail.spbstu.ru/EWS/Exchange.asmx"

// Client signs every request with the keychain credentials (HTTP Basic over TLS); Exchange
// keeps no session, so there is nothing to sign in to.
type Client struct {
	KeychainService string
	// KeychainAccount picks the account of the service to sign in with; "" takes the first one.
	KeychainAccount string
}

const envelope = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
 xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types"
 xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages">
<soap:Header><t:RequestServerVersion Version="Exchange2016"/><t:TimeZoneContext><t:TimeZoneDefinition Id="Russian Standard Time"/></t:TimeZoneContext></soap:Header>
<soap:Body>%s</soap:Body></soap:Envelope>`

// do posts a SOAP body and decodes the response body element into out.
func (c *Client) do(ctx context.Context, body string, out any) error {
	creds, err := keychain.Load(c.KeychainService, c.KeychainAccount)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, Endpoint, strings.NewReader(fmt.Sprintf(envelope, body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.SetBasicAuth(creds.Account, creds.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("mail.spbstu.ru rejected the credentials")
	}
	var env struct {
		Body struct {
			Inner []byte `xml:",innerxml"`
			Fault *struct {
				String string `xml:"faultstring"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("EWS: HTTP %d, unreadable response: %w", resp.StatusCode, err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("EWS fault: %s", env.Body.Fault.String)
	}
	if err := xml.Unmarshal(env.Body.Inner, out); err != nil {
		return err
	}
	return checkResponses(env.Body.Inner)
}

// checkResponses returns the first error any ResponseMessage of a response carries.
func checkResponses(inner []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(inner))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !strings.HasSuffix(se.Name.Local, "ResponseMessage") {
			continue
		}
		for _, a := range se.Attr {
			if a.Name.Local == "ResponseClass" && a.Value == "Error" {
				var msg struct {
					Text string `xml:"MessageText"`
					Code string `xml:"ResponseCode"`
				}
				_ = dec.DecodeElement(&msg, &se)
				return fmt.Errorf("EWS %s: %s", msg.Code, msg.Text)
			}
		}
	}
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Message is a mail item, as listed or read.
type Message struct {
	ID          string       `json:"id"`
	ChangeKey   string       `json:"-"`
	Subject     string       `json:"subject"`
	From        string       `json:"from"`
	To          []string     `json:"to,omitempty"`
	Cc          []string     `json:"cc,omitempty"`
	Received    string       `json:"received"`
	Read        bool         `json:"read"`
	HasAttach   bool         `json:"has_attachments,omitempty"`
	Preview     string       `json:"preview,omitempty"`
	Body        string       `json:"body,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type Attachment struct {
	ID   string `json:"attachment_id"`
	Name string `json:"name"`
	Size int    `json:"size"`
}

type mailbox struct {
	Name  string `xml:"Name"`
	Email string `xml:"EmailAddress"`
}

func (m mailbox) String() string {
	if m.Name == "" || m.Name == m.Email {
		return m.Email
	}
	return fmt.Sprintf("%s <%s>", m.Name, m.Email)
}

type xmlMessage struct {
	ItemID struct {
		ID        string `xml:"Id,attr"`
		ChangeKey string `xml:"ChangeKey,attr"`
	} `xml:"ItemId"`
	Subject     string    `xml:"Subject"`
	From        mailbox   `xml:"From>Mailbox"`
	To          []mailbox `xml:"ToRecipients>Mailbox"`
	Cc          []mailbox `xml:"CcRecipients>Mailbox"`
	Received    string    `xml:"DateTimeReceived"`
	IsRead      bool      `xml:"IsRead"`
	HasAttach   bool      `xml:"HasAttachments"`
	Preview     string    `xml:"Preview"`
	Body        string    `xml:"Body"`
	UniqueBody  string    `xml:"UniqueBody"`
	TextBody    string    `xml:"TextBody"`
	Attachments []struct {
		ID struct {
			ID string `xml:"Id,attr"`
		} `xml:"AttachmentId"`
		Name string `xml:"Name"`
		Size int    `xml:"Size"`
	} `xml:"Attachments>FileAttachment"`
}

func (x xmlMessage) message() Message {
	m := Message{
		ID: x.ItemID.ID, ChangeKey: x.ItemID.ChangeKey, Subject: x.Subject, From: x.From.String(),
		Read: x.IsRead, HasAttach: x.HasAttach, Preview: strings.TrimSpace(x.Preview),
	}
	if t, err := time.Parse(time.RFC3339, x.Received); err == nil {
		m.Received = t.Local().Format("2006-01-02 15:04")
	}
	for _, r := range x.To {
		m.To = append(m.To, r.String())
	}
	for _, r := range x.Cc {
		m.Cc = append(m.Cc, r.String())
	}
	m.Body = strings.TrimSpace(x.TextBody)
	if m.Body == "" {
		m.Body = strings.TrimSpace(x.Body)
	}
	for _, a := range x.Attachments {
		m.Attachments = append(m.Attachments, Attachment{ID: a.ID.ID, Name: a.Name, Size: a.Size})
	}
	return m
}

// Folders that can be listed by name.
var Folders = []string{"inbox", "sentitems", "drafts", "deleteditems", "junkemail"}

// List returns the newest messages of a folder; query, when set, is an Exchange AQS search
// ("from:ivanov", "subject:курсовая", plain words).
func (c *Client) List(ctx context.Context, folder string, limit int, unreadOnly bool, query string) ([]Message, error) {
	if folder == "" {
		folder = "inbox"
	}
	restriction := ""
	if unreadOnly {
		restriction = `<m:Restriction><t:IsEqualTo><t:FieldURI FieldURI="message:IsRead"/><t:FieldURIOrConstant><t:Constant Value="false"/></t:FieldURIOrConstant></t:IsEqualTo></m:Restriction>`
	}
	queryString := ""
	if query != "" {
		queryString = "<m:QueryString>" + esc(query) + "</m:QueryString>"
	}
	body := fmt.Sprintf(`<m:FindItem Traversal="Shallow">
<m:ItemShape><t:BaseShape>IdOnly</t:BaseShape><t:AdditionalProperties>
<t:FieldURI FieldURI="item:Subject"/><t:FieldURI FieldURI="message:From"/><t:FieldURI FieldURI="item:DateTimeReceived"/>
<t:FieldURI FieldURI="message:IsRead"/><t:FieldURI FieldURI="item:HasAttachments"/><t:FieldURI FieldURI="item:Preview"/>
</t:AdditionalProperties></m:ItemShape>
<m:IndexedPageItemView MaxEntriesReturned="%d" Offset="0" BasePoint="Beginning"/>%s
<m:SortOrder><t:FieldOrder Order="Descending"><t:FieldURI FieldURI="item:DateTimeReceived"/></t:FieldOrder></m:SortOrder>
<m:ParentFolderIds><t:DistinguishedFolderId Id="%s"/></m:ParentFolderIds>%s
</m:FindItem>`, limit, restriction, esc(folder), queryString)
	var out struct {
		Items []xmlMessage `xml:"ResponseMessages>FindItemResponseMessage>RootFolder>Items>Message"`
	}
	if err := c.do(ctx, body, &out); err != nil {
		return nil, err
	}
	msgs := make([]Message, 0, len(out.Items))
	for _, x := range out.Items {
		msgs = append(msgs, x.message())
	}
	return msgs, nil
}

// Read returns a whole message as plain text; markRead marks it as read.
func (c *Client) Read(ctx context.Context, id string, markRead bool) (Message, error) {
	body := fmt.Sprintf(`<m:GetItem><m:ItemShape><t:BaseShape>AllProperties</t:BaseShape><t:BodyType>Text</t:BodyType>
<t:AdditionalProperties><t:FieldURI FieldURI="item:TextBody"/></t:AdditionalProperties></m:ItemShape>
<m:ItemIds><t:ItemId Id="%s"/></m:ItemIds></m:GetItem>`, esc(id))
	var out struct {
		Items []xmlMessage `xml:"ResponseMessages>GetItemResponseMessage>Items>Message"`
	}
	if err := c.do(ctx, body, &out); err != nil {
		return Message{}, err
	}
	if len(out.Items) == 0 {
		return Message{}, errors.New("message not found")
	}
	m := out.Items[0].message()
	m.Preview = ""
	if markRead && !m.Read {
		upd := fmt.Sprintf(`<m:UpdateItem ConflictResolution="AutoResolve" MessageDisposition="SaveOnly"><m:ItemChanges><t:ItemChange>
<t:ItemId Id="%s" ChangeKey="%s"/><t:Updates><t:SetItemField><t:FieldURI FieldURI="message:IsRead"/><t:Message><t:IsRead>true</t:IsRead></t:Message></t:SetItemField></t:Updates>
</t:ItemChange></m:ItemChanges></m:UpdateItem>`, esc(m.ID), esc(m.ChangeKey))
		var ignored struct{}
		if err := c.do(ctx, upd, &ignored); err == nil {
			m.Read = true
		}
	}
	return m, nil
}

// Attachment returns a file attachment's name and content.
func (c *Client) Attachment(ctx context.Context, attachmentID string) (string, []byte, error) {
	body := fmt.Sprintf(`<m:GetAttachment><m:AttachmentIds><t:AttachmentId Id="%s"/></m:AttachmentIds></m:GetAttachment>`, esc(attachmentID))
	var out struct {
		Files []struct {
			Name    string `xml:"Name"`
			Content []byte `xml:"Content"`
		} `xml:"ResponseMessages>GetAttachmentResponseMessage>Attachments>FileAttachment"`
	}
	if err := c.do(ctx, body, &out); err != nil {
		return "", nil, err
	}
	if len(out.Files) == 0 {
		return "", nil, errors.New("attachment not found or not a file")
	}
	// Content is base64 in the XML; encoding/xml leaves it as text, so decode here.
	content, err := decodeBase64(out.Files[0].Content)
	return out.Files[0].Name, content, err
}

// Draft describes a message to send or save.
type Draft struct {
	To        []string
	Cc        []string
	Subject   string
	Body      string
	ReplyToID string // reply to this message (quoting it); Subject and To default to the original's
	ReplyAll  bool
	Files     []string // local files to attach
	Send      bool     // false saves to Drafts
}

// MaxAttachBytes bounds the files of one message, below the usual Exchange message size limit.
const MaxAttachBytes = 20 << 20

type attachFile struct {
	Name    string
	Content []byte
}

func readFiles(paths []string) ([]attachFile, error) {
	var out []attachFile
	total := 0
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		total += len(b)
		if total > MaxAttachBytes {
			return nil, fmt.Errorf("attachments exceed %d MB", MaxAttachBytes>>20)
		}
		out = append(out, attachFile{Name: filepath.Base(p), Content: b})
	}
	return out, nil
}

func attachmentsXML(files []attachFile) string {
	var b strings.Builder
	for _, f := range files {
		b.WriteString("<t:FileAttachment><t:Name>" + esc(f.Name) + "</t:Name><t:Content>" +
			base64.StdEncoding.EncodeToString(f.Content) + "</t:Content></t:FileAttachment>")
	}
	return b.String()
}

type itemID struct {
	ID        string `xml:"Id,attr"`
	ChangeKey string `xml:"ChangeKey,attr"`
}

// Submit sends the message, or saves it in Drafts. With files, the message is first saved as a
// draft, the files are attached to it and only then is it sent, which works for replies too.
func (c *Client) Submit(ctx context.Context, d Draft) error {
	files, err := readFiles(d.Files)
	if err != nil {
		return err
	}
	disposition := "SaveOnly"
	if d.Send && len(files) == 0 {
		disposition = "SendAndSaveCopy"
	}
	recipients := func(tag string, addrs []string) string {
		if len(addrs) == 0 {
			return ""
		}
		var b strings.Builder
		b.WriteString("<t:" + tag + ">")
		for _, a := range addrs {
			b.WriteString("<t:Mailbox><t:EmailAddress>" + esc(strings.TrimSpace(a)) + "</t:EmailAddress></t:Mailbox>")
		}
		b.WriteString("</t:" + tag + ">")
		return b.String()
	}
	var item string
	if d.ReplyToID != "" {
		orig, err := c.Read(ctx, d.ReplyToID, false)
		if err != nil {
			return err
		}
		tag := "ReplyToItem"
		if d.ReplyAll {
			tag = "ReplyAllToItem"
		}
		subject := ""
		if d.Subject != "" {
			subject = "<t:Subject>" + esc(d.Subject) + "</t:Subject>"
		}
		item = fmt.Sprintf(`<t:%s>%s%s%s<t:ReferenceItemId Id="%s" ChangeKey="%s"/><t:NewBodyContent BodyType="Text">%s</t:NewBodyContent></t:%s>`,
			tag, subject, recipients("ToRecipients", d.To), recipients("CcRecipients", d.Cc), esc(orig.ID), esc(orig.ChangeKey), esc(d.Body), tag)
	} else {
		if len(d.To) == 0 {
			return errors.New("no recipients")
		}
		item = fmt.Sprintf(`<t:Message><t:Subject>%s</t:Subject><t:Body BodyType="Text">%s</t:Body>%s%s</t:Message>`,
			esc(d.Subject), esc(d.Body), recipients("ToRecipients", d.To), recipients("CcRecipients", d.Cc))
	}
	folder := "drafts"
	if disposition == "SendAndSaveCopy" {
		folder = "sentitems"
	}
	body := fmt.Sprintf(`<m:CreateItem MessageDisposition="%s"><m:SavedItemFolderId><t:DistinguishedFolderId Id="%s"/></m:SavedItemFolderId><m:Items>%s</m:Items></m:CreateItem>`,
		disposition, folder, item)
	var created struct {
		Items []itemID `xml:"ResponseMessages>CreateItemResponseMessage>Items>Message>ItemId"`
	}
	if err := c.do(ctx, body, &created); err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	if len(created.Items) == 0 {
		return errors.New("EWS did not return the draft to attach the files to")
	}
	id := created.Items[0]
	body = fmt.Sprintf(`<m:CreateAttachment><m:ParentItemId Id="%s" ChangeKey="%s"/><m:Attachments>%s</m:Attachments></m:CreateAttachment>`,
		esc(id.ID), esc(id.ChangeKey), attachmentsXML(files))
	var attached struct {
		Root []struct {
			ID        string `xml:"RootItemId,attr"`
			ChangeKey string `xml:"RootItemChangeKey,attr"`
		} `xml:"ResponseMessages>CreateAttachmentResponseMessage>Attachments>FileAttachment>AttachmentId"`
	}
	if err := c.do(ctx, body, &attached); err != nil {
		return err
	}
	if !d.Send {
		return nil
	}
	// Attaching changes the draft, so sending needs its new change key.
	if n := len(attached.Root); n > 0 && attached.Root[n-1].ChangeKey != "" {
		id.ChangeKey = attached.Root[n-1].ChangeKey
	}
	body = fmt.Sprintf(`<m:SendItem SaveItemToFolder="true"><m:ItemIds><t:ItemId Id="%s" ChangeKey="%s"/></m:ItemIds><m:SavedItemFolderId><t:DistinguishedFolderId Id="sentitems"/></m:SavedItemFolderId></m:SendItem>`,
		esc(id.ID), esc(id.ChangeKey))
	var ignored struct{}
	return c.do(ctx, body, &ignored)
}

// Delete moves a message to Deleted Items, from where it can still be restored.
func (c *Client) Delete(ctx context.Context, id string) error {
	body := fmt.Sprintf(`<m:DeleteItem DeleteType="MoveToDeletedItems"><m:ItemIds><t:ItemId Id="%s"/></m:ItemIds></m:DeleteItem>`, esc(id))
	var ignored struct{}
	return c.do(ctx, body, &ignored)
}
