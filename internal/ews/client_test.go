package ews

import (
	"encoding/xml"
	"testing"
)

const findItemResponse = `<m:FindItemResponse xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages" xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
<m:ResponseMessages><m:FindItemResponseMessage ResponseClass="Success"><m:ResponseCode>NoError</m:ResponseCode>
<m:RootFolder><t:Items><t:Message>
<t:ItemId Id="AAA" ChangeKey="CK"/><t:Subject>Re: Долги</t:Subject>
<t:DateTimeReceived>2026-10-19T10:11:00Z</t:DateTimeReceived>
<t:HasAttachments>true</t:HasAttachments><t:IsRead>false</t:IsRead>
<t:From><t:Mailbox><t:Name>Иванова Мария</t:Name><t:EmailAddress>ivanova_m@spbstu.ru</t:EmailAddress></t:Mailbox></t:From>
<t:ToRecipients><t:Mailbox><t:Name>Студент</t:Name><t:EmailAddress>student@edu.spbstu.ru</t:EmailAddress></t:Mailbox></t:ToRecipients>
</t:Message></t:Items></m:RootFolder></m:FindItemResponseMessage></m:ResponseMessages></m:FindItemResponse>`

func TestParseFindItem(t *testing.T) {
	var out struct {
		Items []xmlMessage `xml:"ResponseMessages>FindItemResponseMessage>RootFolder>Items>Message"`
	}
	if err := xml.Unmarshal([]byte(findItemResponse), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("got %d messages", len(out.Items))
	}
	m := out.Items[0].message()
	if m.ID != "AAA" || m.ChangeKey != "CK" || m.Subject != "Re: Долги" || m.Read || !m.HasAttach {
		t.Errorf("unexpected message %+v", m)
	}
	if m.From != "Иванова Мария <ivanova_m@spbstu.ru>" {
		t.Errorf("from = %q", m.From)
	}
	if len(m.To) != 1 || m.To[0] != "Студент <student@edu.spbstu.ru>" {
		t.Errorf("to = %q", m.To)
	}
}

func TestCheckResponsesReportsErrors(t *testing.T) {
	body := `<m:GetItemResponse xmlns:m="m"><m:ResponseMessages><m:GetItemResponseMessage ResponseClass="Error">
<m:MessageText>The specified object was not found in the store.</m:MessageText><m:ResponseCode>ErrorItemNotFound</m:ResponseCode>
</m:GetItemResponseMessage></m:ResponseMessages></m:GetItemResponse>`
	err := checkResponses([]byte(body))
	if err == nil || err.Error() != "EWS ErrorItemNotFound: The specified object was not found in the store." {
		t.Fatalf("err = %v", err)
	}
}

func TestEscape(t *testing.T) {
	if got := esc(`a<b & "c"`); got != "a&lt;b &amp; &#34;c&#34;" {
		t.Errorf("esc = %q", got)
	}
}
