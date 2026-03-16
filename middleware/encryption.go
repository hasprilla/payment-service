package middleware

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/harveyasprilla/sonifoy/payment-service/config"
)

func Decrypt() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Only decrypt POST, PUT, PATCH with body
		if c.Method() != "POST" && c.Method() != "PUT" && c.Method() != "PATCH" {
			return c.Next()
		}

		sessionId := c.Get("X-Session-ID")
		if sessionId == "" {
			// If no session ID, assume it's not encrypted (fallback)
			return c.Next()
		}

		// 1. Get Session Key from Redis
		keyBase64, err := config.RedisClient.Get(config.Ctx, "session:"+sessionId).Result()
		if err != nil {
			log.Printf("[ENCRYPTION] Session not found or expired: %s", sessionId)
			// Return 401 so the EncryptionInterceptor in Flutter clears the session and handshakes again
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Security Session Expired",
				"message": "Session ID not found",
			})
		}

		key, err := base64.StdEncoding.DecodeString(keyBase64)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Internal security error"})
		}

		// 2. Read Body
		encryptedData := string(c.Body())
		
		// Heuristic to check if it's our encrypted format (iv:ciphertext)
		if !strings.Contains(encryptedData, ":") || len(encryptedData) < 20 {
			return c.Next()
		}

		// Remove quotes if Fiber/Middleware already interpreted as string
		encryptedData = strings.Trim(encryptedData, "\"")

		parts := strings.Split(encryptedData, ":")
		if len(parts) != 2 {
			return c.Next()
		}

		iv, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid encryption format (iv)"})
		}

		ciphertext, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid encryption format (ciphertext)"})
		}

		// 3. Decrypt AES-GCM
		block, err := aes.NewCipher(key)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Decryption failed (cipher)"})
		}

		aesGCM, err := cipher.NewGCM(block)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Decryption failed (gcm)"})
		}

		plaintext, err := aesGCM.Open(nil, iv, ciphertext, nil)
		if err != nil {
			log.Printf("[ENCRYPTION] Decryption failed for session %s: %v", sessionId, err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Decryption failed",
				"details": fmt.Sprintf("AES-GCM decryption error: %v", err),
			})
		}

		// 4. Update Request Body with plaintext
		c.Request().SetBody(plaintext)
		c.Request().Header.SetContentType("application/json")

		log.Printf("[ENCRYPTION] Successfully decrypted request for session %s", sessionId)

		return c.Next()
	}
}
