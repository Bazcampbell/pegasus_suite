// util/crypto_test.go

package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func seal(t *testing.T, key []byte, plaintext string) string {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil))
}

func randomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestDecryptRoundTrip(t *testing.T) {
	key := randomKey(t)
	d, err := NewDecrypter(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}

	got, err := d.Decrypt(seal(t, key, "hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Errorf("got %q, want hunter2", got)
	}

	// An unset secret is an empty column, not an error.
	if got, err := d.Decrypt(""); err != nil || got != "" {
		t.Errorf(`Decrypt("") = %q, %v`, got, err)
	}
}

func TestNewRejectsBadKeys(t *testing.T) {
	if _, err := NewDecrypter("not base64!"); err == nil {
		t.Error("expected an error for non-base64")
	}
	if _, err := NewDecrypter(base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Error("expected an error for a 16-byte key")
	}
}

// GCM authenticates, so a wrong key must fail rather than return garbage. This
// is what makes a botched key rotation loud instead of silently corrupting.
func TestDecryptWrongKeyFails(t *testing.T) {
	good, bad := randomKey(t), randomKey(t)

	d, err := NewDecrypter(base64.StdEncoding.EncodeToString(bad))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Decrypt(seal(t, good, "hunter2")); err == nil {
		t.Error("decrypting with the wrong key should fail")
	}
}

func TestDecryptShortCiphertext(t *testing.T) {
	d, err := NewDecrypter(base64.StdEncoding.EncodeToString(randomKey(t)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Decrypt(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Error("expected an error for a ciphertext shorter than the nonce")
	}
}
