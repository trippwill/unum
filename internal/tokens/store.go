package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store struct {
	Path string
	Now  func() time.Time
}

type Token struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	Hash      string    `json:"hash"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"createdAt"`
}

type Registry struct {
	Tokens []Token `json:"tokens"`
}

type CreatedToken struct {
	Token Token
	Raw   string
}

func (s Store) Create(name string) (CreatedToken, error) {
	if strings.TrimSpace(name) == "" {
		return CreatedToken{}, fmt.Errorf("token name is required")
	}
	raw, err := randomToken()
	if err != nil {
		return CreatedToken{}, err
	}
	id, err := randomID()
	if err != nil {
		return CreatedToken{}, err
	}
	reg, err := s.Load()
	if err != nil {
		return CreatedToken{}, err
	}
	token := Token{
		ID:        "tok_" + id,
		Name:      strings.TrimSpace(name),
		Prefix:    raw[:min(len(raw), 18)],
		Hash:      hash(raw),
		CreatedAt: s.now(),
	}
	reg.Tokens = append(reg.Tokens, token)
	if err := s.Save(reg); err != nil {
		return CreatedToken{}, err
	}
	return CreatedToken{Token: token, Raw: raw}, nil
}

func (s Store) Validate(raw string) (bool, error) {
	// ponytail: reads JSON per request; cache hashes when token lookup shows up in profiles.
	reg, err := s.Load()
	if err != nil {
		return false, err
	}
	candidate := hash(raw)
	for _, token := range reg.Tokens {
		if token.Revoked {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(token.Hash)) == 1 {
			return true, nil
		}
	}
	return false, nil
}

func (s Store) Load() (Registry, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return Registry{}, nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("read token registry %s: %w", s.Path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return Registry{}, fmt.Errorf("parse token registry %s: %w", s.Path, err)
	}
	return reg, nil
}

func (s Store) Save(reg Registry) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token registry: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("create token registry directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".inference-tokens-*.json")
	if err != nil {
		return fmt.Errorf("create temporary token registry: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary token registry: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temporary token registry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary token registry: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("replace token registry %s: %w", s.Path, err)
	}
	return nil
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate inference token: %w", err)
	}
	return "unum_sk_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate inference token id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
