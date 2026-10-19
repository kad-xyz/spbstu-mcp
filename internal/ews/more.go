package ews

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Folder struct {
	ID     string `json:"folder_id"`
	Name   string `json:"name"`
	Total  int    `json:"total"`
	Unread int    `json:"unread"`
}

// Folders lists every mail folder of the mailbox with its message counts.
func (c *Client) Folders(ctx context.Context) ([]Folder, error) {
	body := `<m:FindFolder Traversal="Deep"><m:FolderShape><t:BaseShape>Default</t:BaseShape>
<t:AdditionalProperties><t:FieldURI FieldURI="folder:FolderClass"/></t:AdditionalProperties></m:FolderShape>
<m:ParentFolderIds><t:DistinguishedFolderId Id="msgfolderroot"/></m:ParentFolderIds></m:FindFolder>`
	var out struct {
		Folders []struct {
			ID struct {
				ID string `xml:"Id,attr"`
			} `xml:"FolderId"`
			Name   string `xml:"DisplayName"`
			Class  string `xml:"FolderClass"`
			Total  int    `xml:"TotalCount"`
			Unread int    `xml:"UnreadCount"`
		} `xml:"ResponseMessages>FindFolderResponseMessage>RootFolder>Folders>Folder"`
	}
	if err := c.do(ctx, body, &out); err != nil {
		return nil, err
	}
	folders := make([]Folder, 0, len(out.Folders))
	for _, f := range out.Folders {
		// Mail folders only: Exchange also keeps conversation settings, Yammer, RSS and sync logs here.
		if f.Class != "" && f.Class != "IPF.Note" {
			continue
		}
		folders = append(folders, Folder{ID: f.ID.ID, Name: f.Name, Total: f.Total, Unread: f.Unread})
	}
	return folders, nil
}

// folderRef is a distinguished folder name (inbox, deleteditems, …) or a folder_id from Folders.
func folderRef(folder string) string {
	for _, d := range append(Folders, "archive", "msgfolderroot") {
		if strings.EqualFold(folder, d) {
			return `<t:DistinguishedFolderId Id="` + strings.ToLower(folder) + `"/>`
		}
	}
	return `<t:FolderId Id="` + esc(folder) + `"/>`
}

// Move moves messages to a folder.
func (c *Client) Move(ctx context.Context, ids []string, folder string) error {
	var items strings.Builder
	for _, id := range ids {
		items.WriteString(`<t:ItemId Id="` + esc(id) + `"/>`)
	}
	body := `<m:MoveItem><m:ToFolderId>` + folderRef(folder) + `</m:ToFolderId><m:ItemIds>` + items.String() + `</m:ItemIds></m:MoveItem>`
	var ignored struct{}
	return c.do(ctx, body, &ignored)
}

// changeKeys returns the current ChangeKey of each item, which Exchange requires for updates.
func (c *Client) changeKeys(ctx context.Context, ids []string) (map[string]string, error) {
	var items strings.Builder
	for _, id := range ids {
		items.WriteString(`<t:ItemId Id="` + esc(id) + `"/>`)
	}
	body := `<m:GetItem><m:ItemShape><t:BaseShape>IdOnly</t:BaseShape></m:ItemShape><m:ItemIds>` + items.String() + `</m:ItemIds></m:GetItem>`
	var out struct {
		IDs []struct {
			ID        string `xml:"Id,attr"`
			ChangeKey string `xml:"ChangeKey,attr"`
		} `xml:"ResponseMessages>GetItemResponseMessage>Items>Message>ItemId"`
	}
	if err := c.do(ctx, body, &out); err != nil {
		return nil, err
	}
	keys := map[string]string{}
	for _, x := range out.IDs {
		keys[x.ID] = x.ChangeKey
	}
	return keys, nil
}

// MarkRead marks messages read or unread.
func (c *Client) MarkRead(ctx context.Context, ids []string, read bool) error {
	keys, err := c.changeKeys(ctx, ids)
	if err != nil {
		return err
	}
	var changes strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&changes, `<t:ItemChange><t:ItemId Id="%s" ChangeKey="%s"/><t:Updates><t:SetItemField><t:FieldURI FieldURI="message:IsRead"/>`+
			`<t:Message><t:IsRead>%t</t:IsRead></t:Message></t:SetItemField></t:Updates></t:ItemChange>`, esc(id), esc(keys[id]), read)
	}
	body := `<m:UpdateItem ConflictResolution="AutoResolve" MessageDisposition="SaveOnly"><m:ItemChanges>` +
		changes.String() + `</m:ItemChanges></m:UpdateItem>`
	var ignored struct{}
	return c.do(ctx, body, &ignored)
}

type Event struct {
	Subject   string `json:"subject"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Location  string `json:"location,omitempty"`
	Organizer string `json:"organizer,omitempty"`
	AllDay    bool   `json:"all_day,omitempty"`
}

// Calendar lists the mailbox calendar's events between from and to.
func (c *Client) Calendar(ctx context.Context, from, to time.Time) ([]Event, error) {
	body := fmt.Sprintf(`<m:FindItem Traversal="Shallow"><m:ItemShape><t:BaseShape>IdOnly</t:BaseShape><t:AdditionalProperties>
<t:FieldURI FieldURI="item:Subject"/><t:FieldURI FieldURI="calendar:Start"/><t:FieldURI FieldURI="calendar:End"/>
<t:FieldURI FieldURI="calendar:Location"/><t:FieldURI FieldURI="calendar:Organizer"/><t:FieldURI FieldURI="calendar:IsAllDayEvent"/>
</t:AdditionalProperties></m:ItemShape><m:CalendarView MaxEntriesReturned="200" StartDate="%s" EndDate="%s"/>
<m:ParentFolderIds><t:DistinguishedFolderId Id="calendar"/></m:ParentFolderIds></m:FindItem>`,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	var out struct {
		Items []struct {
			Subject   string  `xml:"Subject"`
			Start     string  `xml:"Start"`
			End       string  `xml:"End"`
			Location  string  `xml:"Location"`
			Organizer mailbox `xml:"Organizer>Mailbox"`
			AllDay    bool    `xml:"IsAllDayEvent"`
		} `xml:"ResponseMessages>FindItemResponseMessage>RootFolder>Items>CalendarItem"`
	}
	if err := c.do(ctx, body, &out); err != nil {
		return nil, err
	}
	local := func(s string) string {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return s
		}
		return t.Local().Format("2006-01-02 15:04")
	}
	events := make([]Event, 0, len(out.Items))
	for _, e := range out.Items {
		events = append(events, Event{Subject: e.Subject, Start: local(e.Start), End: local(e.End), Location: e.Location,
			Organizer: e.Organizer.String(), AllDay: e.AllDay})
	}
	return events, nil
}
