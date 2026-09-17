// util/crypto.go

package util

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
)

type Decrypter struct {
	aead cipher.AEAD
}

func NewDecrypter(base64Key string) (*Decrypter, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("encryption key: invalid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key: must decode to 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &Decrypter{aead: aead}, nil
}

func (d *Decrypter) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	if len(raw) < d.aead.NonceSize() {
		return "", errors.New("ciphertext too short")
	}

	nonce, body := raw[:d.aead.NonceSize()], raw[d.aead.NonceSize():]
	plaintext, err := d.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
