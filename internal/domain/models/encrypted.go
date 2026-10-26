package models

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"gorm.io/gorm/schema"
)

// encryptedPrefix marks a column value written by EncryptedSerializer.
// Values without it are legacy plaintext and are returned unchanged.
const encryptedPrefix = "enc:v1:"

// ErrEncryptionKeyMissing is returned when an encrypted value is read but
// CONFIG_ENCRYPTION_KEY is not set.
var ErrEncryptionKeyMissing = errors.New("CONFIG_ENCRYPTION_KEY is required to decrypt stored secrets")

func init() {
	schema.RegisterSerializer("encrypted", EncryptedSerializer{})
}

// EncryptedSerializer encrypts string columns at rest with AES-256-GCM when
// CONFIG_ENCRYPTION_KEY is set. Without the key, values are stored as
// plaintext as before. Plaintext values already in the database are still
// read, and are encrypted the next time the row is saved.
type EncryptedSerializer struct{}

// encryptionKey derives the AES-256 key from CONFIG_ENCRYPTION_KEY, or
// returns nil when encryption is not configured.
func encryptionKey() []byte {
	secret := os.Getenv("CONFIG_ENCRYPTION_KEY")
	if secret == "" {
		return nil
	}
	key := sha256.Sum256([]byte("taunewlety-config-encryption:" + secret))
	return key[:]
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Scan decrypts a stored value into the string field.
func (EncryptedSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	var stored string
	switch v := dbValue.(type) {
	case nil:
	case string:
		stored = v
	case []byte:
		stored = string(v)
	default:
		return fmt.Errorf("encrypted serializer: unsupported value type %T", dbValue)
	}

	plain, err := decryptValue(stored)
	if err != nil {
		return fmt.Errorf("decrypting %s: %w", field.Name, err)
	}
	field.ReflectValueOf(ctx, dst).SetString(plain)
	return nil
}

// Value encrypts the string field for storage.
func (EncryptedSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	plain, _ := fieldValue.(string)
	return encryptValue(plain)
}

func encryptValue(plain string) (string, error) {
	key := encryptionKey()
	if key == nil || plain == "" {
		return plain, nil
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptValue(stored string) (string, error) {
	if !strings.HasPrefix(stored, encryptedPrefix) {
		return stored, nil // legacy plaintext
	}
	key := encryptionKey()
	if key == nil {
		return "", ErrEncryptionKeyMissing
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encryptedPrefix))
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("wrong CONFIG_ENCRYPTION_KEY or corrupted value")
	}
	return string(plain), nil
}
