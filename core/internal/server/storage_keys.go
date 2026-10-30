package server

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"os"
	"syscall"
)

// Read only operator-configured key files, with bounded input and no path/key
// contents in errors. Rotating keys requires a restart; no online secret API.
func readStorageKey(path string) ([]byte, error) {
	// Linux is the supported runtime. Nonblocking open prevents FIFO hangs;
	// NOFOLLOW prevents replacement of an operator key by a symlink.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("storage encryption key unavailable: %w", os.ErrNotExist)
		}
		return nil, fmt.Errorf("storage encryption key unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("storage encryption key must be a private regular file")
	}
	key, err := io.ReadAll(io.LimitReader(file, 33))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("storage encryption key must contain exactly 32 raw bytes")
	}
	return key, nil
}
func storageCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("storage encryption key must contain exactly 32 raw bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("invalid storage encryption key")
	}
	return cipher.NewGCM(block)
}
func (s *Server) initializeStorageCiphers(key []byte) error {
	if len(s.config.Storage.PreviousEncryptionKeyFiles) > 8 {
		return fmt.Errorf("at most eight previous storage encryption keys are supported")
	}
	primary, err := storageCipher(key)
	if err != nil {
		return err
	}
	s.aead = primary
	for _, path := range s.config.Storage.PreviousEncryptionKeyFiles {
		key, err := readStorageKey(path)
		if err != nil {
			return err
		}
		previous, err := storageCipher(key)
		if err != nil {
			return err
		}
		s.previousAEADs = append(s.previousAEADs, previous)
	}
	return nil
}
