package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordAlgorithm  = "pbkdf2_sha256"
	passwordIterations = 600000
	passwordSaltLen    = 16
	passwordKeyLen     = 32
	minIterations      = 10000
	maxIterations      = 2000000
)

var (
	ErrWeakPassword      = errors.New("密码至少需要 12 个字符")
	ErrInvalidHashFormat = errors.New("密码哈希格式非法")
)

// HashPassword 生成 pbkdf2_sha256$iter$salt$hash 格式的哈希。
func HashPassword(password string) (string, error) {
	if len([]rune(password)) < 12 {
		return "", ErrWeakPassword
	}
	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s", passwordAlgorithm, passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword 校验密码。密码错误返回 (false, nil)，哈希损坏返回错误。
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordAlgorithm {
		return false, ErrInvalidHashFormat
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < minIterations || iterations > maxIterations {
		return false, ErrInvalidHashFormat
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false, ErrInvalidHashFormat
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false, ErrInvalidHashFormat
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
