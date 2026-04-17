package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

type FieldCipher struct {
	aead cipher.AEAD
}

func NewFieldCipher(secret string) (FieldCipher, error) {
	key := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return FieldCipher{}, fmt.Errorf("new aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return FieldCipher{}, fmt.Errorf("new gcm cipher: %w", err)
	}

	return FieldCipher{aead: aead}, nil
}

func (c FieldCipher) Encrypt(value string) (string, error) {
	plaintext := strings.TrimSpace(value)
	if plaintext == "" {
		return "", fmt.Errorf("password is required")
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}

	ciphertext := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func (c FieldCipher) Decrypt(value string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("decode secret envelope: %w", err)
	}

	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", fmt.Errorf("secret envelope is invalid")
	}

	plaintext, err := c.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("open secret envelope: %w", err)
	}

	return string(plaintext), nil
}
