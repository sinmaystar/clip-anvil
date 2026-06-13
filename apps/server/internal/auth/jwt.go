package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func SignToken(accountID pgtype.UUID, secret string, expireHours int) (string, error) {
	if !accountID.Valid {
		return "", errors.New("account id is invalid")
	}

	claims := jwt.RegisteredClaims{
		Subject:   uuidToString(accountID),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func VerifyToken(tokenStr string, secret string) (pgtype.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return pgtype.UUID{}, err
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return pgtype.UUID{}, errors.New("invalid token")
	}

	var accountID pgtype.UUID
	if err := accountID.Scan(claims.Subject); err != nil {
		return pgtype.UUID{}, err
	}
	return accountID, nil
}

func uuidToString(id pgtype.UUID) string {
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		id.Bytes[0:4],
		id.Bytes[4:6],
		id.Bytes[6:8],
		id.Bytes[8:10],
		id.Bytes[10:16],
	)
}
