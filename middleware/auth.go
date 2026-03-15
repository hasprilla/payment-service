package middleware

import (
	"crypto/rsa"
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

var (
	verifyKey *rsa.PublicKey
)

func init() {
	pubKeyPEM := os.Getenv("JWT_PUBLIC_KEY")
	if pubKeyPEM != "" {
		key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pubKeyPEM))
		if err != nil {
			fmt.Printf("Warning: Failed to parse JWT_PUBLIC_KEY: %v\n", err)
		} else {
			verifyKey = key
		}
	}
}

func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing authorization header",
			})
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid authorization format",
			})
		}

		tokenString := parts[1]
		
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			if verifyKey == nil {
				return nil, fmt.Errorf("JWT public key not configured")
			}
			return verifyKey, nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
				"details": err.Error(),
			})
		}

		claims := token.Claims.(jwt.MapClaims)
		c.Locals("user_id", claims["sub"])
		c.Locals("email", claims["email"])
		c.Locals("role", claims["role"])

		return c.Next()
	}
}
