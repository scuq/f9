// Package cred is a passphrase-locked secret store for external-source
// credentials (HTTPS auth tokens, basic-auth passwords, mTLS key material).
// A key is derived from a user passphrase with argon2id and secrets are sealed
// with NaCl secretbox. The passphrase is never persisted; the store opens
// locked and is unlocked on demand.
//
// This is deliberately separate from SSH/session auth, which is never stored
// (ADR-0005). See ADR-0007.
package cred

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/nacl/secretbox"
	"gopkg.in/yaml.v3"
)

const (
	magic     = "f9-cred-v1"
	saltLen   = 16
	keyLen    = 32
	argonTime = 1
	argonMem  = 64 * 1024 // KiB (64 MiB)
	argonProc = 4
)

// Sentinel errors.
var (
	ErrLocked     = errors.New("cred: store is locked")
	ErrNotInit    = errors.New("cred: no passphrase set")
	ErrInit       = errors.New("cred: passphrase already set")
	ErrPassphrase = errors.New("cred: wrong passphrase")
	ErrNotFound   = errors.New("cred: secret not found")
)

type fileModel struct {
	Salt  string            `yaml:"salt"`
	Check string            `yaml:"check"`
	Items map[string]string `yaml:"items,omitempty"`
}

// Store is a file-backed, passphrase-locked secret store.
type Store struct {
	path string

	mu    sync.Mutex
	salt  []byte
	check string            // base64(sealed magic) — verifies the passphrase
	items map[string]string // id -> base64(sealed secret)
	key   *[keyLen]byte     // nil when locked
}

// Open loads the store (or returns an empty, uninitialized one if the file is
// absent). It does not require a passphrase; the store opens locked.
func Open(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("cred: read: %w", err)
	}
	var fm fileModel
	if err := yaml.Unmarshal(data, &fm); err != nil {
		return nil, fmt.Errorf("cred: parse: %w", err)
	}
	if fm.Salt != "" {
		salt, err := base64.StdEncoding.DecodeString(fm.Salt)
		if err != nil {
			return nil, fmt.Errorf("cred: salt: %w", err)
		}
		s.salt = salt
	}
	s.check = fm.Check
	if fm.Items != nil {
		s.items = fm.Items
	}
	return s, nil
}

// Initialized reports whether a passphrase has been set.
func (s *Store) Initialized() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.salt) > 0 && s.check != ""
}

// Locked reports whether the store needs unlocking before Put/Get.
func (s *Store) Locked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key == nil
}

// SetPassphrase performs first-time setup. Fails if already initialized.
func (s *Store) SetPassphrase(pass string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.salt) > 0 && s.check != "" {
		return ErrInit
	}
	if pass == "" {
		return errors.New("cred: empty passphrase")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("cred: salt: %w", err)
	}
	key := deriveKey(pass, salt)
	check, err := seal(key, []byte(magic))
	if err != nil {
		return err
	}
	s.salt = salt
	s.check = check
	s.key = key
	return s.saveLocked()
}

// Unlock derives the key from the passphrase and verifies it against the check
// blob.
func (s *Store) Unlock(pass string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.salt) == 0 || s.check == "" {
		return ErrNotInit
	}
	key := deriveKey(pass, s.salt)
	plain, ok := openBox(key, s.check)
	if !ok || subtle.ConstantTimeCompare(plain, []byte(magic)) != 1 {
		return ErrPassphrase
	}
	s.key = key
	return nil
}

// Lock zeroes the in-memory key.
func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != nil {
		for i := range s.key {
			s.key[i] = 0
		}
		s.key = nil
	}
}

// Put stores (encrypts) a secret. Requires an unlocked store.
func (s *Store) Put(id, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return ErrLocked
	}
	blob, err := seal(s.key, []byte(secret))
	if err != nil {
		return err
	}
	s.items[id] = blob
	return s.saveLocked()
}

// Get decrypts a secret. Requires an unlocked store.
func (s *Store) Get(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return "", ErrLocked
	}
	blob, ok := s.items[id]
	if !ok {
		return "", ErrNotFound
	}
	plain, ok := openBox(s.key, blob)
	if !ok {
		return "", errors.New("cred: decrypt failed")
	}
	return string(plain), nil
}

// Has reports whether a secret id exists (no passphrase needed).
func (s *Store) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[id]
	return ok
}

// Delete removes a secret id (no passphrase needed).
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return nil
	}
	delete(s.items, id)
	return s.saveLocked()
}

func deriveKey(pass string, salt []byte) *[keyLen]byte {
	raw := argon2.IDKey([]byte(pass), salt, argonTime, argonMem, argonProc, keyLen)
	var k [keyLen]byte
	copy(k[:], raw)
	return &k
}

func seal(key *[keyLen]byte, plaintext []byte) (string, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("cred: nonce: %w", err)
	}
	out := secretbox.Seal(nonce[:], plaintext, &nonce, key)
	return base64.StdEncoding.EncodeToString(out), nil
}

func openBox(key *[keyLen]byte, blob string) ([]byte, bool) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil || len(raw) < 24 {
		return nil, false
	}
	var nonce [24]byte
	copy(nonce[:], raw[:24])
	return secretbox.Open(nil, raw[24:], &nonce, key)
}

func (s *Store) saveLocked() error {
	fm := fileModel{
		Salt:  base64.StdEncoding.EncodeToString(s.salt),
		Check: s.check,
		Items: s.items,
	}
	data, err := yaml.Marshal(&fm)
	if err != nil {
		return fmt.Errorf("cred: marshal: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cred: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".f9cred-*")
	if err != nil {
		return fmt.Errorf("cred: temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cred: write: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cred: chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cred: close: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cred: rename: %w", err)
	}
	return nil
}
