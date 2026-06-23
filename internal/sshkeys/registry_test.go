package sshkeys

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestStoreAddAuthorizeAndRevoke(t *testing.T) {
	store := Store{
		Path: filepath.Join(t.TempDir(), "authorized-clients.json"),
		Now:  func() time.Time { return time.Unix(100, 0) },
	}
	pub := testAuthorizedKey(t)

	client, err := store.Add("laptop", AdminRole, pub)
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "laptop" || client.Role != AdminRole || client.Revoked {
		t.Fatalf("unexpected client: %+v", client)
	}

	key, _, _, _, err := ssh.ParseAuthorizedKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok, err := store.Authorize(key); err != nil || !ok || got.ID != client.ID {
		t.Fatalf("Authorize() = %+v, %v, %v", got, ok, err)
	}

	if err := store.Revoke(client.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Authorize(key); err != nil || ok {
		t.Fatalf("Authorize after revoke = %v, %v", ok, err)
	}
}

func TestStoreRejectsDuplicateActiveKey(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	pub := testAuthorizedKey(t)
	if _, err := store.Add("one", AdminRole, pub); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add("two", AdminRole, pub); err == nil {
		t.Fatal("duplicate active key was accepted")
	}
}

func TestStoreAcceptsSingleKeyAuthorizedKeysFile(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	pub := testAuthorizedKey(t)
	authorizedKeys := append([]byte("# laptop key\n\n"), pub...)

	if _, err := store.Add("laptop", AdminRole, authorizedKeys); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsMultipleAuthorizedKeys(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	authorizedKeys := append(testAuthorizedKey(t), testAuthorizedKey(t)...)

	if _, err := store.Add("laptop", AdminRole, authorizedKeys); err == nil {
		t.Fatal("multiple keys were accepted")
	}
}

func TestStoreAddsAuthorizedKeysFile(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	keys := append([]byte("# admin keys\n"), testAuthorizedKey(t)...)
	keys = append(keys, testAuthorizedKey(t)...)

	clients, skipped, err := store.AddAuthorizedKeys("admin", AdminRole, keys)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 || len(clients) != 2 {
		t.Fatalf("clients=%d skipped=%d", len(clients), skipped)
	}
	if clients[0].Name != "admin-1" || clients[1].Name != "admin-2" {
		t.Fatalf("unexpected names: %+v", clients)
	}
}

func TestStoreAddAuthorizedKeysSkipsDuplicates(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	key := testAuthorizedKey(t)
	if _, err := store.Add("laptop", AdminRole, key); err != nil {
		t.Fatal(err)
	}
	clients, skipped, err := store.AddAuthorizedKeys("admin", AdminRole, key)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 || len(clients) != 0 {
		t.Fatalf("clients=%d skipped=%d", len(clients), skipped)
	}
}

func TestStoreUsesPrivateRegistryPermissions(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	if _, err := store.Add("laptop", AdminRole, testAuthorizedKey(t)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("registry permissions = %o", got)
	}
}

func testAuthorizedKey(t *testing.T) []byte {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return ssh.MarshalAuthorizedKey(sshPub)
}
