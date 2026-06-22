package tokens

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateStoresHashOnlyAndValidate(t *testing.T) {
	store := Store{
		Path: filepath.Join(t.TempDir(), "tokens.json"),
		Now:  func() time.Time { return time.Unix(100, 0) },
	}
	created, err := store.Create("editor")
	if err != nil {
		t.Fatal(err)
	}
	if created.Raw == "" || created.Token.Hash == "" || created.Token.Hash == created.Raw {
		t.Fatalf("bad token: %+v", created)
	}

	ok, err := store.Validate(created.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("created token did not validate")
	}
	if ok, err := store.Validate("unum_sk_wrong"); err != nil || ok {
		t.Fatalf("wrong token validated: %v %v", ok, err)
	}

	data, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(created.Raw)) {
		t.Fatal("raw token was stored")
	}
}

func TestTokenRegistryPermissions(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "tokens.json")}
	if _, err := store.Create("editor"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions = %o", got)
	}
}
