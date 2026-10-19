package moodle

import "testing"

func TestText(t *testing.T) {
	cases := map[string]string{
		"<p>Экзамен&nbsp;в <b>10:00</b></p><p>Аудитория 101</p>": "Экзамен в 10:00\nАудитория 101",
		"line<br/>break":            "line\nbreak",
		"<script>x()</script>plain": "plain",
		"":                          "",
	}
	for in, want := range cases {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAuthFailure(t *testing.T) {
	if !(&Error{ErrorCode: "invalidtoken"}).authFailure() {
		t.Error("invalidtoken must trigger a new sign-in")
	}
	if (&Error{ErrorCode: "invalidparameter"}).authFailure() {
		t.Error("invalidparameter must not trigger a new sign-in")
	}
}
