package store

import (
	"encoding/hex"
	"testing"
)

func testKey() string {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return hex.EncodeToString(key)
}

func TestEncryptorRoundTrip(t *testing.T) {
	enc, err := NewEncryptor(testKey())
	if err != nil {
		t.Fatal(err)
	}

	original := `{"host":"smtp.example.com","pass":"s3cret"}`
	encrypted, err := enc.Encrypt(original)
	if err != nil {
		t.Fatal(err)
	}

	if !IsEncrypted(encrypted) {
		t.Error("expected encrypted prefix")
	}
	if encrypted == original {
		t.Error("encrypted should differ from original")
	}

	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != original {
		t.Errorf("got %q, want %q", decrypted, original)
	}
}

func TestEncryptorDecryptPlaintext(t *testing.T) {
	enc, err := NewEncryptor(testKey())
	if err != nil {
		t.Fatal(err)
	}

	plain := `{"url":"https://hooks.slack.com/test"}`
	result, err := enc.Decrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if result != plain {
		t.Errorf("plaintext passthrough failed: got %q", result)
	}
}

func TestEncryptorBadKey(t *testing.T) {
	_, err := NewEncryptor("tooshort")
	if err == nil {
		t.Error("expected error for short key")
	}

	_, err = NewEncryptor("not-hex-at-all-but-long-enough-to-be-64-chars-if-we-keep-going!!")
	if err == nil {
		t.Error("expected error for non-hex key")
	}
}

func TestEncryptorUniqueCiphertexts(t *testing.T) {
	enc, err := NewEncryptor(testKey())
	if err != nil {
		t.Fatal(err)
	}

	a, err := enc.Encrypt("same")
	if err != nil {
		t.Fatalf("first encrypt: %v", err)
	}
	b, err := enc.Encrypt("same")
	if err != nil {
		t.Fatalf("second encrypt: %v", err)
	}
	if a == b {
		t.Error("two encryptions of same plaintext should produce different ciphertexts")
	}
}
