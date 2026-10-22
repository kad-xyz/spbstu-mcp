package accounts

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestActiveAdoptsFirstAndMovesState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"dl.token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Registry{Dir: dir, First: func() (string, error) { return "Ivanov.II@edu.spbstu.ru", nil }}
	got, err := r.Active()
	if err != nil || got != "ivanov.ii@edu.spbstu.ru" {
		t.Fatalf("Active() = %q, %v", got, err)
	}
	if v := r.Store(got).Get("dl.token"); v != "x" {
		t.Fatalf("state was not moved, dl.token = %q", v)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !os.IsNotExist(err) {
		t.Fatal("old state.json is left behind")
	}

	// Later calls read the saved choice and never ask First again.
	r.First = func() (string, error) { t.Fatal("First called again"); return "", nil }
	if got, _ := r.Active(); got != "ivanov.ii@edu.spbstu.ru" {
		t.Fatalf("Active() = %q", got)
	}
}

func TestSwitchAndRemove(t *testing.T) {
	r := &Registry{Dir: t.TempDir(), First: func() (string, error) { return "ivanov.ii@edu.spbstu.ru", nil }}
	if _, err := r.Active(); err != nil {
		t.Fatal(err)
	}
	if err := r.SetActive("petrov.pp@edu.spbstu.ru"); err == nil {
		t.Fatal("switched to an account that was not added")
	}
	if err := r.Add("petrov.pp@edu.spbstu.ru"); err != nil {
		t.Fatal(err)
	}
	if err := r.Store("petrov.pp@edu.spbstu.ru").Set("k", "petrov"); err != nil {
		t.Fatal(err)
	}
	if v := r.Store("ivanov.ii@edu.spbstu.ru").Get("k"); v != "" {
		t.Fatalf("stores are shared: %q", v)
	}
	if err := r.SetActive("petrov.pp@edu.spbstu.ru"); err != nil {
		t.Fatal(err)
	}
	if got, _ := (&Registry{Dir: r.Dir}).Active(); got != "petrov.pp@edu.spbstu.ru" {
		t.Fatalf("active account did not survive a restart: %q", got)
	}
	if err := r.Remove("petrov.pp@edu.spbstu.ru"); err == nil {
		t.Fatal("removed the active account")
	}
	if err := r.Remove("ivanov.ii@edu.spbstu.ru"); err != nil {
		t.Fatal(err)
	}
	if list, _ := r.List(); !slices.Equal(list, []string{"petrov.pp@edu.spbstu.ru"}) {
		t.Fatalf("List() = %v", list)
	}
}

func TestInvalidAccount(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	for _, a := range []string{"", "..", "../x", "a/b", `a\b`, ".hidden"} {
		if err := r.Add(a); err == nil {
			t.Errorf("Add(%q) accepted", a)
		}
	}
}
