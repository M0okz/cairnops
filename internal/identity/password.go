package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMemory      = 64 * 1024
	passwordIterations  = 3
	passwordParallelism = 4
	passwordSaltLength  = 16
	passwordKeyLength   = 32
)

var errInvalidPasswordHash = errors.New("invalid password hash")

type passwordParameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var defaultPasswordParameters = passwordParameters{
	memory:      passwordMemory,
	iterations:  passwordIterations,
	parallelism: passwordParallelism,
	saltLength:  passwordSaltLength,
	keyLength:   passwordKeyLength,
}

var dummyPasswordHash = func() string {
	parameters := defaultPasswordParameters
	salt := []byte("cairnops-no-user")
	hash := argon2.IDKey([]byte("not-a-real-user-password"), salt, parameters.iterations, parameters.memory, parameters.parallelism, parameters.keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		parameters.memory,
		parameters.iterations,
		parameters.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}()

func hashPassword(password string) (string, error) {
	parameters := defaultPasswordParameters
	salt := make([]byte, parameters.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, parameters.iterations, parameters.memory, parameters.parallelism, parameters.keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		parameters.memory,
		parameters.iterations,
		parameters.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errInvalidPasswordHash
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return false, errInvalidPasswordHash
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, errInvalidPasswordHash
	}
	if memory < 8*1024 || memory > 1024*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 {
		return false, errInvalidPasswordHash
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, errInvalidPasswordHash
	}
	expected, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false, errInvalidPasswordHash
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
