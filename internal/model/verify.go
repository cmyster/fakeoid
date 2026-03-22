package model

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// VerifySHA256 computes the SHA256 hash of the file at filePath and compares
// it to expectedHash. Returns true if they match, false otherwise.
// Returns an error if the file cannot be read.
func VerifySHA256(filePath, expectedHash string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	return actual == expectedHash, nil
}
