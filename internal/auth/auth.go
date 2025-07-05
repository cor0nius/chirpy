package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	pwdByte := []byte(password)
	pdwHash, err := bcrypt.GenerateFromPassword(pwdByte, 10)
	if err != nil {
		return "", err
	}
	pwdHashString := string(pdwHash)
	return pwdHashString, nil
}

func CheckPasswordHash(password, hash string) error {
	pwdByte := []byte(password)
	hashByte := []byte(hash)
	err := bcrypt.CompareHashAndPassword(hashByte, pwdByte)
	if err != nil {
		return err
	}
	return nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.RegisteredClaims{Issuer: "chirpy",
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour).UTC()),
			Subject:   userID.String()})
	signedToken, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := jwt.RegisteredClaims{}
	token, err := jwt.NewParser().ParseWithClaims(tokenString, &claims, func(tkn *jwt.Token) (any, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	token.Claims = jwt.Claims(claims)
	tokenSubject, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}
	userID, err := uuid.Parse(tokenSubject)
	if err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	bearer := headers.Get("Authorization")
	if bearer == "" {
		return "", errors.New("no appropriate header or header is empty")
	}
	bearerToken, ok := strings.CutPrefix(bearer, "Bearer ")
	if !ok {
		return "", errors.New("no Bearer token in header")
	}
	return bearerToken, nil
}

func MakeRefreshToken() (string, error) {
	tokenByte := make([]byte, 32)
	_, _ = rand.Read(tokenByte)
	tokenString := hex.EncodeToString(tokenByte)
	return tokenString, nil
}

func GetAPIKey(headers http.Header) (string, error) {
	apiKey := headers.Get("Authorization")
	if apiKey == "" {
		return "", errors.New("no appropriate header or header is empty")
	}
	key, ok := strings.CutPrefix(apiKey, "ApiKey ")
	if !ok {
		return "", errors.New("no ApiKey token in header")
	}
	return key, nil
}
