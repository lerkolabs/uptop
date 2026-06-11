package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"

	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// kcMockStore implements only what keyCache and userInvalidatingStore touch;
// any other Store method panics via the embedded nil interface.
type kcMockStore struct {
	store.Store
	users []models.User
	err   error
}

func (m *kcMockStore) GetAllUsers() ([]models.User, error) { return m.users, m.err }
func (m *kcMockStore) DeleteUser(int) error                { return nil }

func testKey(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return string(gossh.MarshalAuthorizedKey(sk)), sk
}

func TestKeyCache_AllowsKnownDeniesUnknown(t *testing.T) {
	authorized, known := testKey(t)
	_, unknown := testKey(t)
	kc := newKeyCache(&kcMockStore{users: []models.User{{PublicKey: authorized}}})

	if !kc.IsAllowed(known) {
		t.Error("known key denied")
	}
	if kc.IsAllowed(unknown) {
		t.Error("unknown key allowed")
	}
}

func TestKeyCache_RetainsKeysOnRefreshError(t *testing.T) {
	authorized, known := testKey(t)
	ms := &kcMockStore{users: []models.User{{PublicKey: authorized}}}
	kc := newKeyCache(ms)

	if !kc.IsAllowed(known) {
		t.Fatal("known key denied on first refresh")
	}

	// DB goes down and the cache goes stale: a transient error must not lock
	// every admin out — the previous key set stays in effect.
	ms.err = errors.New("db down")
	kc.mu.Lock()
	kc.updated = time.Now().Add(-time.Hour)
	kc.mu.Unlock()

	if !kc.IsAllowed(known) {
		t.Error("transient refresh error locked out a previously valid key")
	}
}

func TestKeyCache_FailsClosedAfterInvalidate(t *testing.T) {
	authorized, known := testKey(t)
	ms := &kcMockStore{users: []models.User{{PublicKey: authorized}}}
	kc := newKeyCache(ms)

	if !kc.IsAllowed(known) {
		t.Fatal("known key denied on first refresh")
	}

	// Revocation happened (Invalidate) and the DB is unreachable for the
	// re-read: the revoked key must NOT keep working off the stale cache.
	ms.err = errors.New("db down")
	kc.Invalidate()

	if kc.IsAllowed(known) {
		t.Error("revoked key still allowed while DB is down — fails open")
	}
}

func TestUserInvalidatingStore_DeleteDropsKeyCache(t *testing.T) {
	authorized, known := testKey(t)
	ms := &kcMockStore{users: []models.User{{PublicKey: authorized}}}
	kc := newKeyCache(ms)
	s := &userInvalidatingStore{Store: ms, kc: kc}

	if !kc.IsAllowed(known) {
		t.Fatal("known key denied on first refresh")
	}

	// Revoke the user; DB unreachable immediately after. The cached key must
	// be gone the moment the delete returns.
	if err := s.DeleteUser(1); err != nil {
		t.Fatal(err)
	}
	ms.users = nil
	ms.err = errors.New("db down")

	if kc.IsAllowed(known) {
		t.Error("deleted user's key still allowed from stale cache")
	}
}
