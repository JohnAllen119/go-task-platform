package task

import (
	"crypto/rand"
	"encoding/hex"
)

func generateLeaseToken() (string, error) {
	token := make([]byte, 16)
	_, err := rand.Read(token)
	if err != nil {
		return "", err
	}
	s := hex.EncodeToString(token)
	return s, nil
}
