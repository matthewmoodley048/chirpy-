package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	hashedPassword, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", err
	}

	return hashedPassword, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	passwordValidity, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, err
	}

	return passwordValidity, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	jwtClaims := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject:   userID.String(),
	}

	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)

	signedToken, err := newToken.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	type uClaims struct {
		jwt.RegisteredClaims
	}

	token, err := jwt.ParseWithClaims(tokenString, &uClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.UUID{}, err
	}

	claims, ok := token.Claims.(*uClaims)
	if !ok {
		return uuid.UUID{}, errors.New("unknown claims type")
	}

	return uuid.Parse(claims.Subject)
}

func GetBearerToken(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")
	if len(authHeader) <= 0 {
		return authHeader, errors.New("no authorization header found")
	}

	authHeader = strings.TrimPrefix(authHeader, "Bearer ")

	if len(authHeader) <= 0 {
		return authHeader, errors.New("no token found")
	}
	return authHeader, nil
}

func MakeRefreshToken() string {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	return hex.EncodeToString(key)
}

func GetAPIKey(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")

	if len(authHeader) <= 0 {
		return authHeader, errors.New("no authorization header found")
	}

	authHeader = strings.TrimPrefix(authHeader, "ApiKey ")

	if len(authHeader) <= 0 {
		return authHeader, errors.New("no token found")
	}
	return authHeader, nil
}
