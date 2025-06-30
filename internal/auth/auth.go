package auth

import (
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

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.RegisteredClaims{Issuer: "chirpy",
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
			Subject:   userID.String()})
	signedToken, err := token.SignedString(tokenSecret)
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token := &jwt.Token{}
	token, err := jwt.ParseWithClaims(tokenString, token.Claims, func(tkn *jwt.Token) (interface{}, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, err
	}
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
