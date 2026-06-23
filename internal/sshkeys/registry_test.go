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

func TestStoreRejectsAuthorizedKeysOptions(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	authorizedKeys := append([]byte(`from="10.0.0.0/8" `), testAuthorizedKey(t)...)

	if _, err := store.Add("laptop", AdminRole, authorizedKeys); err == nil {
		t.Fatal("authorized_keys options were accepted")
	}
	if _, _, err := store.AddAuthorizedKeys("admin", AdminRole, authorizedKeys); err == nil {
		t.Fatal("authorized_keys options were accepted for bulk import")
	}
}

func TestStoreRejectsAuthorizedKeyCertificates(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "authorized-clients.json")}
	cert := testAuthorizedCertificate(t)

	if _, err := store.Add("laptop", AdminRole, cert); err == nil {
		t.Fatal("ssh certificate was accepted")
	}
	if _, _, err := store.AddAuthorizedKeys("admin", AdminRole, cert); err == nil {
		t.Fatal("ssh certificate was accepted for bulk import")
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

func testAuthorizedCertificate(t *testing.T) []byte {
	t.Helper()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caPub, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	caKey, err := ssh.NewPublicKey(caPub)
	if err != nil {
		t.Fatal(err)
	}
	caSigner, err := ssh.NewSignerFromSigner(caPrivate)
	if err != nil {
		t.Fatal(err)
	}
	cert := &ssh.Certificate{
		Key:             key,
		Serial:          1,
		CertType:        ssh.UserCert,
		KeyId:           "test",
		ValidPrincipals: []string{"tripp"},
		ValidBefore:     ssh.CertTimeInfinity,
		SignatureKey:    caKey,
	}
	if err := cert.SignCert(rand.Reader, caSigner); err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.NewSignerFromSigner(private); err != nil {
		t.Fatal(err)
	}
	return ssh.MarshalAuthorizedKey(cert)
}
