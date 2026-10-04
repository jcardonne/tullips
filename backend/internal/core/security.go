package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

func SignUser(secret, user string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(user))
	return hex.EncodeToString(m.Sum(nil))
}
func validSignature(secret, user, sig string) bool {
	if secret == "" || user == "" {
		return false
	}
	a, e := hex.DecodeString(sig)
	if e != nil {
		return false
	}
	b, _ := hex.DecodeString(SignUser(secret, user))
	return hmac.Equal(a, b)
}
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "tlp_" + base64.RawURLEncoding.EncodeToString(b)
}
func Encrypt(secret, plaintext string) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("ENCRYPTION_KEY must contain at least 32 characters")
	}
	key := sha256.Sum256([]byte(secret))
	block, e := aes.NewCipher(key[:])
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	return base64.RawStdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}
func Decrypt(secret, ciphertext string) (string, error) {
	key := sha256.Sum256([]byte(secret))
	block, e := aes.NewCipher(key[:])
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	b, e := base64.RawStdEncoding.DecodeString(ciphertext)
	if e != nil || len(b) < g.NonceSize() {
		return "", errors.New("invalid ciphertext")
	}
	v, e := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
	return string(v), e
}
