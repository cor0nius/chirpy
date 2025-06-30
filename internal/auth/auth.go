package auth

import (
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
