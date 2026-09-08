package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordTime    uint32 = 3
	passwordMemory  uint32 = 64 * 1024
	passwordThreads uint8  = 4
	passwordSaltLen        = 16
	passwordKeyLen  uint32 = 32
)

var errInvalidPasswordHash = errors.New("invalid password hash")

type passwordParams struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
}

var defaultPasswordParams = passwordParams{
	time:    passwordTime,
	memory:  passwordMemory,
	threads: passwordThreads,
	keyLen:  passwordKeyLen,
}

func hashPassword(password string) (string, error) {
	return hashPasswordWithParams(password, defaultPasswordParams)
}

func hashPasswordWithParams(password string, params passwordParams) (string, error) {
	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, params.keyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.memory,
		params.time,
		params.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errInvalidPasswordHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errInvalidPasswordHash
	}
	if parts[2] != fmt.Sprintf("v=%d", version) {
		return false, errInvalidPasswordHash
	}

	var params passwordParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.time, &params.threads); err != nil {
		return false, errInvalidPasswordHash
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", params.memory, params.time, params.threads) {
		return false, errInvalidPasswordHash
	}
	if params.memory < 8*1024 || params.memory > 256*1024 || params.time < 1 || params.time > 10 || params.threads < 1 || params.threads > 16 {
		return false, errInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, errInvalidPasswordHash
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false, errInvalidPasswordHash
	}
	params.keyLen = uint32(len(expected))

	actual := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, params.keyLen)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func consumePasswordWork(password string) {
	salt := make([]byte, passwordSaltLen)
	actual := argon2.IDKey(
		[]byte(password),
		salt,
		defaultPasswordParams.time,
		defaultPasswordParams.memory,
		defaultPasswordParams.threads,
		defaultPasswordParams.keyLen,
	)
	subtle.ConstantTimeCompare(actual, make([]byte, passwordKeyLen))
}
