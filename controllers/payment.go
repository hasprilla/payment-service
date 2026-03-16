package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/harveyasprilla/sonifoy/payment-service/config"
	"github.com/harveyasprilla/sonifoy/payment-service/models"
	"github.com/harveyasprilla/sonifoy/payment-service/utils"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"
)

// SeedPackages inserts the default star packages if they don't exist
func SeedPackages(db *gorm.DB) {
	packages := []models.StarPackage{
		{Type: "bronze", Name: "1 Bronce", Price: 2000, Currency: "COP", Amount: 1},
		{Type: "silver", Name: "5 Plata", Price: 8000, Currency: "COP", Amount: 5},
		{Type: "gold", Name: "10 Oro", Price: 15000, Currency: "COP", Amount: 10},
	}

	for _, pkg := range packages {
		var existing models.StarPackage
		if err := db.Where("type = ?", pkg.Type).First(&existing).Error; err != nil {
			db.Create(&pkg)
		} else {
			db.Model(&existing).Updates(pkg)
		}
	}
	log.Println("Successfully synchronized star packages")
}

// GetPackages returns available star packages
func GetPackages(c *fiber.Ctx) error {
	db := config.DB
	var packages []models.StarPackage

	if err := db.Find(&packages).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch packages"})
	}

	return c.JSON(packages)
}

// BuyStars processes a mocked credit card payment
func BuyStars(c *fiber.Ctx) error {
	// 1. Get User ID from JWT context (Set by Protected middleware)
	userIdLocal := c.Locals("user_id")
	if userIdLocal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var userID uint
	switch v := userIdLocal.(type) {
	case float64:
		userID = uint(v)
	case uint:
		userID = v
	case int:
		userID = uint(v)
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid user context"})
	}

	var req models.PurchaseRequest
	body := c.Body()
	log.Printf("[DEBUG] BuyStars request body: %s", string(body))

	if err := c.BodyParser(&req); err != nil {
		log.Printf("[ERROR] BuyStars body parser error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request format", "details": err.Error()})
	}

	log.Printf("[DEBUG] BuyStars parsed request: %+v", req)

	db := config.DB

	// 2. Fetch the requested package
	var pkg models.StarPackage
	if err := db.First(&pkg, req.StarPackageID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Star package not found"})
	}

	// 3. Mock Payment Validation (Credit/Debit Card)
	if len(req.CardNumber) < 4 || req.CVV == "" || req.Expiry == "" {
		log.Printf("[ERROR] BuyStars validation failed: CardNumber len=%d, CVV=%s, Expiry=%s", len(req.CardNumber), req.CVV, req.Expiry)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid card details"})
	}

	// Simulate payment processing delay...
	// In reality we'd call Stripe, PayPal, Wompi, etc.
	log.Printf("Processing payment of %f %s for package %s...", pkg.Price, pkg.Currency, pkg.Name)

	// 4. Create Transaction Record
	tx := db.Begin()
	transaction := models.PaymentTransaction{
		UserID:        userID,
		StarPackageID: pkg.ID,
		Amount:        pkg.Price,
		Currency:      pkg.Currency,
		Status:        "completed",
		PaymentMethod: "credit_card",
		RecipientArtistID: req.RecipientArtistID,
	}

	if err := tx.Create(&transaction).Error; err != nil {
		log.Printf("[ERROR] BuyStars failed to create transaction: %v", err)
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create transaction record"})
	}
	tx.Commit()
	log.Printf("[DEBUG] BuyStars transaction created with ID: %d", transaction.ID)

	// 5. Publish to Kafka to notify Wallet Service (Async)
	log.Printf("[DEBUG] BuyStars publishing event to Kafka...")
	event := map[string]interface{}{
		"event":          "star_purchase",
		"user_id":        userID,
		"package_id":     pkg.ID,
		"star_type":      pkg.Type,
		"star_amount":    pkg.Amount,
		"transaction_id": transaction.ID,
		"recipient_id":   req.RecipientArtistID,
	}
	eventData, _ := json.Marshal(event)
	
	// We use the same Kafka utility as the other services
	utils.PublishEvent(c.Context(), fmt.Sprintf("%d", userID), eventData)

	// 6. Notify Wallet Service via Internal HTTP request for synchronous balance update
	walletURL := os.Getenv("WALLET_SERVICE_URL")
	if walletURL == "" {
		walletURL = "http://wallet-service.railway.internal:8080" // default for Railway
	}

	targetUserID := userID
	if req.RecipientArtistID > 0 {
		targetUserID = req.RecipientArtistID
	}

	addStarsReq := map[string]interface{}{
		"user_id":                targetUserID,
		"star_type":              pkg.Type,
		"amount":                 pkg.Amount,
		"payment_transaction_id": transaction.ID,
		"donor_id":               userID,
	}

	reqBody, _ := json.Marshal(addStarsReq)
	resp, err := http.Post(walletURL+"/internal/wallet/add-stars", "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("Failed to synchronously notify wallet service: %v", err)
	} else {
		log.Println("Wallet service notified successfully via HTTP")
	}
	if resp != nil {
		resp.Body.Close()
	}

	return c.JSON(fiber.Map{
		"message": "Payment successful",
		"transaction": transaction,
		"package": pkg,
	})
}
