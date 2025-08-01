package tools

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/rnd-varnion/utils/authentication"
	"github.com/rnd-varnion/utils/permission"
)

type Claims struct {
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"`
	LoginTime int64     `json:"login_time"`
	jwt.RegisteredClaims
}

type PermissionClaims struct {
	Data []*PermissionToken `json:"data"`
	jwt.RegisteredClaims
}

type InternalClaims struct {
	Iss  string `json:"iss"`
	Aud  string `json:"aud"`
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type PermissionToken struct {
	AppName string  `json:"app_name"`
	Menus   []*Menu `json:"menus"`
}

type Menu struct {
	MenuName   string    `json:"menu_name"`
	Permission []*string `json:"permission"`
}

func ValidateAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		secret := os.Getenv(authentication.ACCESS_SECRET_KEY)
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func ValidatePermissionToken(tokenString string) (*PermissionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &PermissionClaims{}, func(token *jwt.Token) (interface{}, error) {
		secret := os.Getenv(permission.PERMISSION_SECRET_KEY)
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*PermissionClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func GenerateInternalToken(Iss, Aud string) (string, error) {
	expiryStr := os.Getenv(authentication.JWT_EXPIRY_INTERNAL_SECONDS)
	expirySeconds, err := strconv.Atoi(expiryStr)
	if err != nil {
		expirySeconds = 15 // fallback default
	}

	expirationTime := time.Now().Add(time.Duration(expirySeconds) * time.Second)

	claims := &InternalClaims{
		Iss:  Iss,
		Aud:  Aud,
		Role: "internal",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret := os.Getenv(authentication.INTERNAL_ACCESS_SECRET_KEY)
	if secret == "" {
		return "", fmt.Errorf("INTERNAL_ACCESS_SECRET_KEY is not sets")
	}
	return token.SignedString([]byte(secret))
}

func ValidateInternalToken(tokenString string) (*InternalClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &InternalClaims{}, func(token *jwt.Token) (interface{}, error) {
		secret := os.Getenv(authentication.INTERNAL_ACCESS_SECRET_KEY)
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*InternalClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
