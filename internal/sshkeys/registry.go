package sshkeys

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	AdminRole = "admin"
)

type Client struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	PublicKey  string     `json:"publicKey"`
	Role       string     `json:"role"`
	Revoked    bool       `json:"revoked"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
}

type Registry struct {
	Clients []Client `json:"clients"`
}

type Store struct {
	Path string
	Now  func() time.Time
}

func (s Store) Add(name, role string, publicKey []byte) (Client, error) {
	if strings.TrimSpace(name) == "" {
		return Client{}, fmt.Errorf("name is required")
	}
	if role == "" {
		role = AdminRole
	}
	if role != AdminRole {
		return Client{}, fmt.Errorf("unsupported role %q", role)
	}

	normalized, err := normalizePublicKey(publicKey)
	if err != nil {
		return Client{}, err
	}

	// ponytail: whole-file registry write; add file locking if concurrent admins matter.
	reg, err := s.Load()
	if err != nil {
		return Client{}, err
	}
	for _, client := range reg.Clients {
		if client.PublicKey == normalized && !client.Revoked {
			return Client{}, fmt.Errorf("public key is already registered as %s", client.ID)
		}
	}

	id, err := randomID()
	if err != nil {
		return Client{}, err
	}

	client := Client{
		ID:        id,
		Name:      strings.TrimSpace(name),
		PublicKey: normalized,
		Role:      role,
		CreatedAt: s.now(),
	}
	reg.Clients = append(reg.Clients, client)
	if err := s.Save(reg); err != nil {
		return Client{}, err
	}
	return client, nil
}

func (s Store) Revoke(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("client id is required")
	}
	reg, err := s.Load()
	if err != nil {
		return err
	}
	for i := range reg.Clients {
		if reg.Clients[i].ID == id {
			reg.Clients[i].Revoked = true
			return s.Save(reg)
		}
	}
	return fmt.Errorf("ssh client %q not found", id)
}

func (s Store) Authorize(publicKey ssh.PublicKey) (Client, bool, error) {
	normalized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey)))
	reg, err := s.Load()
	if err != nil {
		return Client{}, false, err
	}
	for _, client := range reg.Clients {
		if client.PublicKey == normalized && !client.Revoked {
			return client, true, nil
		}
	}
	return Client{}, false, nil
}

func (s Store) Load() (Registry, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return Registry{}, nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("read ssh registry %s: %w", s.Path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return Registry{}, fmt.Errorf("parse ssh registry %s: %w", s.Path, err)
	}
	return reg, nil
}

func (s Store) Save(reg Registry) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ssh registry: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("create ssh registry directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".authorized-clients-*.json")
	if err != nil {
		return fmt.Errorf("create temporary ssh registry: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary ssh registry: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temporary ssh registry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary ssh registry: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("replace ssh registry %s: %w", s.Path, err)
	}
	return nil
}

func normalizePublicKey(data []byte) (string, error) {
	key, _, _, rest, err := ssh.ParseAuthorizedKey(data)
	if err != nil {
		return "", fmt.Errorf("parse public key: %w", err)
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return "", fmt.Errorf("public key file must contain exactly one key")
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), nil
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func randomID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate ssh client id: %w", err)
	}
	return "sshclient_" + hex.EncodeToString(buf), nil
}
