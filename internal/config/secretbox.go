package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const secretBoxKeySize = 32

// sealedPrefix marks a file written by SecretBox so a plaintext file is never
// mistaken for ciphertext (or the other way round).
const sealedPrefix = "xcsealed1:"

// SecretBox seals per-user source registries with AES-256-GCM. The key lives
// in its own 0600 file under the plugin state directory. This keeps provider
// passwords out of anything the plugin serves or logs; it cannot hide them
// from someone with root on the host, because the plugin must decrypt them to
// open a stream.
type SecretBox struct {
	keyPath string
	once    sync.Once
	aead    cipher.AEAD
	err     error
}

func NewSecretBox(keyPath string) *SecretBox {
	return &SecretBox{keyPath: keyPath}
}

func (b *SecretBox) init() {
	b.once.Do(func() {
		key, err := loadOrCreateSecretKey(b.keyPath)
		if err != nil {
			b.err = err
			return
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			b.err = fmt.Errorf("secret box cipher: %w", err)
			return
		}
		b.aead, b.err = cipher.NewGCM(block)
	})
}

func (b *SecretBox) Seal(plaintext []byte) ([]byte, error) {
	b.init()
	if b.err != nil {
		return nil, b.err
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secret box nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, plaintext, nil)
	return append([]byte(sealedPrefix), sealed...), nil
}

func (b *SecretBox) Open(data []byte) ([]byte, error) {
	b.init()
	if b.err != nil {
		return nil, b.err
	}
	if !strings.HasPrefix(string(data), sealedPrefix) {
		return nil, errors.New("secret box: data is not sealed")
	}
	data = data[len(sealedPrefix):]
	if len(data) < b.aead.NonceSize() {
		return nil, errors.New("secret box: sealed data is truncated")
	}
	nonce, ciphertext := data[:b.aead.NonceSize()], data[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("secret box: %w", err)
	}
	return plaintext, nil
}

func loadOrCreateSecretKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != secretBoxKeySize {
			return nil, fmt.Errorf("secret key %s has %d bytes, want %d", path, len(key), secretBoxKeySize)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	key = make([]byte, secretBoxKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create secret key directory: %w", err)
	}
	// O_EXCL: if another process created the key first, use theirs.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return loadOrCreateSecretKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create secret key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		return nil, fmt.Errorf("write secret key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close secret key: %w", err)
	}
	return key, nil
}
