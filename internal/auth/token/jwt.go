// Package token signs and parses the HS256 JSON Web Tokens the server hands
// out: the session tokens of package usersession and billing's order event
// tickets. It checks only the signature and the expiry; what a token grants
// is for the package that issued it to decide.
package token

import (
	"github.com/golang-jwt/jwt/v5"
)

// Option jwt additional data
type Option struct {
	Key string
	Val any
}

// WithOption returns Option with key-value pairs
func WithOption(key string, val any) Option {
	return Option{
		Key: key,
		Val: val,
	}
}

// NewJwtToken Generate and return jwt token with given data.
func NewJwtToken(secretKey string, iat, seconds int64, opt ...Option) (string, error) {
	claims := make(jwt.MapClaims)
	claims["exp"] = iat + seconds
	claims["iat"] = iat

	for _, v := range opt {
		claims[v.Key] = v.Val
	}

	token := jwt.New(jwt.SigningMethodHS256)
	token.Claims = claims
	return token.SignedString([]byte(secretKey))
}

// ParseJwtToken parses a token this package issued and returns its claims.
// Only HS256 is accepted, the algorithm NewJwtToken signs with, so a token
// declaring another algorithm ("none", an RSA one) is refused before its
// signature is looked at; and an expiry is required, since every token this
// package issues carries one, so a forged token cannot live forever by
// leaving it out.
func ParseJwtToken(tokenString, secretKey string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(secretKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, jwt.ErrTokenInvalidId
	}
	return claims, nil
}
