package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

type PasswordHasher struct {
	iterations int
	saltSize   int
	keySize    int
}

func NewPasswordHasher(iterations, saltSize, keySize int) PasswordHasher {
	return PasswordHasher{
		iterations: iterations,
		saltSize:   saltSize,
		keySize:    keySize,
	}
}

func (h PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.saltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key := pbkdf2SHA256([]byte(password), salt, h.iterations, h.keySize)
	return fmt.Sprintf(
		"pbkdf2$sha256$%d$%s$%s",
		h.iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func (h PasswordHasher) Compare(encodedHash, password string) error {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 5 || parts[0] != "pbkdf2" || parts[1] != "sha256" {
		return fmt.Errorf("unsupported password hash format")
	}

	iterations, err := strconv.Atoi(parts[2])
	if err != nil {
		return fmt.Errorf("invalid password hash iterations")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return fmt.Errorf("invalid password hash salt")
	}

	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return fmt.Errorf("invalid password hash checksum")
	}

	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return fmt.Errorf("password mismatch")
	}

	return nil
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	hLen := 32
	numBlocks := (keyLength + hLen - 1) / hLen
	derivedKey := make([]byte, 0, numBlocks*hLen)

	for blockIndex := 1; blockIndex <= numBlocks; blockIndex++ {
		block := pbkdf2Block(password, salt, iterations, blockIndex)
		derivedKey = append(derivedKey, block...)
	}

	return derivedKey[:keyLength]
}

func pbkdf2Block(password, salt []byte, iterations, blockIndex int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	mac.Write([]byte{
		byte(blockIndex >> 24),
		byte(blockIndex >> 16),
		byte(blockIndex >> 8),
		byte(blockIndex),
	})
	u := mac.Sum(nil)
	result := append([]byte(nil), u...)

	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range result {
			result[j] ^= u[j]
		}
	}

	return result
}
