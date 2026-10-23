package openedu

import "testing"

func TestThreadID(t *testing.T) {
	for in, want := range map[string]string{
		"https://courses.openedu.ru/courses/course-v1:spbstu+WEBPYT+fall_2026/discussion/forum/course/threads/6a707596d7a8be0026ce3ff5": "6a707596d7a8be0026ce3ff5",
		"6a707596d7a8be0026ce3ff5": "6a707596d7a8be0026ce3ff5",
	} {
		if got, err := threadID(in); err != nil || got != want {
			t.Errorf("%q → %q, %v", in, got, err)
		}
	}
	if _, err := threadID("course"); err == nil {
		t.Error("want an error without an id")
	}
}
