package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type researchflowIDGenerator struct{}

func (researchflowIDGenerator) NewRef(ctx context.Context, purpose string) (string, error) {
	if ctx == nil || strings.TrimSpace(purpose) != purpose || len(purpose) == 0 || len(purpose) > 32 {
		return "", errors.New("Researchflow reference purpose is invalid")
	}
	for _, char := range purpose {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_') {
			return "", errors.New("Researchflow reference purpose is invalid")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate Researchflow reference entropy: %w", err)
	}
	return purpose + "_" + hex.EncodeToString(entropy[:]), nil
}
