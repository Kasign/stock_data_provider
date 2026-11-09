package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2idVersion     = 19
	maxPasswordBytes    = 1024
	maxEncodedHashBytes = 512
	maxArgonMemoryKiB   = 256 * 1024
	maxArgonIterations  = 10
	maxArgonParallelism = 8
	minArgonSaltBytes   = 16
	maxArgonSaltBytes   = 64
	minArgonKeyBytes    = 16
	maxArgonKeyBytes    = 64
)

var (
	errInvalidPassword     = errors.New("password input is invalid")
	errInvalidPasswordHash = errors.New("password hash is invalid")
)

// Argon2idParams defines the cost parameters for a password hash.
// Memory is measured in KiB. No implicit production defaults are selected:
// callers must supply parameters that have been benchmarked on target hardware.
type Argon2idParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// Argon2idHasher hashes and verifies passwords using the PHC Argon2id format.
type Argon2idHasher struct {
	params Argon2idParams
	random io.Reader
}

// NewArgon2idHasher constructs a hasher with explicitly selected parameters.
func NewArgon2idHasher(params Argon2idParams) (*Argon2idHasher, error) {
	if err := validateArgon2idParams(params); err != nil {
		return nil, err
	}
	return &Argon2idHasher{params: params, random: rand.Reader}, nil
}

func validateArgon2idParams(params Argon2idParams) error {
	if params.Parallelism == 0 || params.Parallelism > maxArgonParallelism ||
		params.Memory < 8*uint32(params.Parallelism) || params.Memory > maxArgonMemoryKiB ||
		params.Iterations == 0 || params.Iterations > maxArgonIterations ||
		params.SaltLength < minArgonSaltBytes || params.SaltLength > maxArgonSaltBytes ||
		params.KeyLength < minArgonKeyBytes || params.KeyLength > maxArgonKeyBytes {
		return errors.New("Argon2id parameters are outside supported safety bounds")
	}
	return nil
}

// Hash returns a PHC-encoded Argon2id hash. The returned string contains the
// random salt and derived key; callers must never log it or expose it publicly.
func (h *Argon2idHasher) Hash(password []byte) (string, error) {
	if h == nil || h.random == nil || validateArgon2idParams(h.params) != nil {
		return "", errors.New("Argon2id hasher is not configured")
	}
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return "", errInvalidPassword
	}

	salt := make([]byte, h.params.SaltLength)
	if _, err := io.ReadFull(h.random, salt); err != nil {
		return "", errors.New("secure password salt generation failed")
	}
	key := argon2.IDKey(password, salt, h.params.Iterations, h.params.Memory, h.params.Parallelism, h.params.KeyLength)
	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2idVersion, h.params.Memory, h.params.Iterations, h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
	return encoded, nil
}

// Verify checks a password against a PHC-encoded Argon2id hash.
func (h *Argon2idHasher) Verify(encoded string, password []byte) (bool, error) {
	if h == nil || h.random == nil || validateArgon2idParams(h.params) != nil {
		return false, errors.New("Argon2id hasher is not configured")
	}
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return false, errInvalidPassword
	}
	params, salt, expected, err := parseArgon2idHash(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey(password, salt, params.Iterations, params.Memory, params.Parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// NeedsRehash reports whether an encoded hash uses parameters different from
// this hasher's configured target parameters.
func (h *Argon2idHasher) NeedsRehash(encoded string) (bool, error) {
	if h == nil || validateArgon2idParams(h.params) != nil {
		return false, errors.New("Argon2id hasher is not configured")
	}
	params, _, _, err := parseArgon2idHash(encoded)
	if err != nil {
		return false, err
	}
	return params != h.params, nil
}

func parseArgon2idHash(encoded string) (Argon2idParams, []byte, []byte, error) {
	if len(encoded) == 0 || len(encoded) > maxEncodedHashBytes {
		return Argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || !strings.HasPrefix(parts[2], "v=") {
		return Argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	versionPart := strings.TrimPrefix(parts[2], "v=")
	version, err := strconv.ParseUint(versionPart, 10, 32)
	if err != nil || version != argon2idVersion {
		return Argon2idParams{}, nil, nil, errInvalidPasswordHash
	}

	var params Argon2idParams
	for _, item := range strings.Split(parts[3], ",") {
		keyValue := strings.SplitN(item, "=", 2)
		if len(keyValue) != 2 {
			return Argon2idParams{}, nil, nil, errInvalidPasswordHash
		}
		value, parseErr := strconv.ParseUint(keyValue[1], 10, 32)
		if parseErr != nil {
			return Argon2idParams{}, nil, nil, errInvalidPasswordHash
		}
		switch keyValue[0] {
		case "m":
			if params.Memory != 0 {
				return Argon2idParams{}, nil, nil, errInvalidPasswordHash
			}
			params.Memory = uint32(value)
		case "t":
			if params.Iterations != 0 {
				return Argon2idParams{}, nil, nil, errInvalidPasswordHash
			}
			params.Iterations = uint32(value)
		case "p":
			if params.Parallelism != 0 || value > 255 {
				return Argon2idParams{}, nil, nil, errInvalidPasswordHash
			}
			params.Parallelism = uint8(value)
		default:
			return Argon2idParams{}, nil, nil, errInvalidPasswordHash
		}
	}
	// Bound attacker-controlled cost parameters before invoking Argon2.
	if params.Parallelism == 0 || params.Parallelism > maxArgonParallelism ||
		params.Memory < 8*uint32(params.Parallelism) || params.Memory > maxArgonMemoryKiB ||
		params.Iterations == 0 || params.Iterations > maxArgonIterations {
		return Argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[4])
	key, keyErr := base64.RawStdEncoding.DecodeString(parts[5])
	if saltErr != nil || keyErr != nil || len(salt) < minArgonSaltBytes || len(salt) > maxArgonSaltBytes ||
		len(key) < minArgonKeyBytes || len(key) > maxArgonKeyBytes {
		return Argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	return params, salt, key, nil
}
