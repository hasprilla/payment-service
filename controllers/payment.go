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
	"strconv"
	"strings"
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

	log.Printf("[DEBUG] BuyStars completed successfully for user %d", userID)
	return c.JSON(fiber.Map{
		"message": "Payment successful",
		"transaction": transaction,
		"package": pkg,
	})
}

// CreateMercadoPagoPreference creates a preference for MercadoPago Checkout Pro
func CreateMercadoPagoPreference(c *fiber.Ctx) error {
	userIdLocal := c.Locals("user_id")
	if userIdLocal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var userID uint
	switch v := userIdLocal.(type) {
	case float64: userID = uint(v)
	case uint: userID = v
	case int: userID = uint(v)
	default: return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid user context"})
	}

	var req models.PurchaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request format"})
	}

	db := config.DB
	var pkg models.StarPackage
	if err := db.First(&pkg, req.StarPackageID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Star package not found"})
	}

	accessToken := os.Getenv("MERCADOPAGO_ACCESS_TOKEN")
	if accessToken == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "MercadoPago not configured"})
	}

	// Create preference via API
	prefReq := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"id":          strconv.Itoa(int(pkg.ID)),
				"title":       pkg.Name,
				"description": fmt.Sprintf("Compra de %d estrellas %s", pkg.Amount, pkg.Type),
				"quantity":    1,
				"unit_price":  pkg.Price,
				"currency_id": pkg.Currency,
			},
		},
		"payer": map[string]interface{}{
			"email": "test_user_79224068@testuser.com", // In production use actual user email
		},
		"back_urls": map[string]interface{}{
			"success": "sonifoy://payment/success",
			"pending": "sonifoy://payment/pending",
			"failure": "sonifoy://payment/failure",
		},
		"auto_return": "approved",
		"notification_url": os.Getenv("MERCADOPAGO_WEBHOOK_URL"),
		"external_reference": fmt.Sprintf("%d-%d-%d", userID, pkg.ID, req.RecipientArtistID),
	}

	jsonData, _ := json.Marshal(prefReq)
	client := &http.Client{}
	mpReq, _ := http.NewRequest("POST", "https://api.mercadopago.com/checkout/preferences", bytes.NewBuffer(jsonData))
	mpReq.Header.Set("Authorization", "Bearer "+accessToken)
	mpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(mpReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect to MercadoPago"})
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return c.JSON(result)
}

// MercadoPagoWebhook handles notifications from MercadoPago
func MercadoPagoWebhook(c *fiber.Ctx) error {
	var notification map[string]interface{}
	if err := c.BodyParser(&notification); err != nil {
		return c.SendStatus(fiber.StatusOK)
	}

	log.Printf("[WEBHOOK] Received MercadoPago notification: %+v", notification)

	action, _ := notification["action"].(string)
	if action != "payment.created" {
		return c.SendStatus(fiber.StatusOK)
	}

	data, ok := notification["data"].(map[string]interface{})
	if !ok {
		return c.SendStatus(fiber.StatusOK)
	}

	paymentID, _ := data["id"].(string)
	accessToken := os.Getenv("MERCADOPAGO_ACCESS_TOKEN")

	// Verify payment status
	client := &http.Client{}
	req, _ := http.NewRequest("GET", "https://api.mercadopago.com/v1/payments/"+paymentID, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return c.SendStatus(fiber.StatusOK)
	}
	defer resp.Body.Close()

	var payment map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&payment)

	status, _ := payment["status"].(string)
	if status == "approved" {
		extRef, _ := payment["external_reference"].(string)
		parts := strings.Split(extRef, "-")
		if len(parts) >= 2 {
			userID, _ := strconv.Atoi(parts[0])
			pkgID, _ := strconv.Atoi(parts[1])
			recipientID := 0
			if len(parts) >= 3 {
				recipientID, _ = strconv.Atoi(parts[2])
			}

			// Process successful payment (create transaction, notify wallet)
			processSuccessfulMPPayment(uint(userID), uint(pkgID), uint(recipientID), paymentID)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

func processSuccessfulMPPayment(userID, pkgID, recipientID uint, mpPaymentID string) {
	db := config.DB
	var pkg models.StarPackage
	if err := db.First(&pkg, pkgID).Error; err != nil {
		return
	}

	// Double check if already processed
	var existing models.PaymentTransaction
	if err := db.Where("payment_method = ? AND status = ?", "mercadopago:"+mpPaymentID, "completed").First(&existing).Error; err == nil {
		return
	}

	transaction := models.PaymentTransaction{
		UserID:            userID,
		StarPackageID:     pkgID,
		Amount:            pkg.Price,
		Currency:          pkg.Currency,
		Status:            "completed",
		PaymentMethod:     "mercadopago:" + mpPaymentID,
		RecipientArtistID: recipientID,
	}

	db.Create(&transaction)

	// Publish Kafka
	event := map[string]interface{}{
		"event":          "star_purchase",
		"user_id":        userID,
		"package_id":     pkg.ID,
		"star_type":      pkg.Type,
		"star_amount":    pkg.Amount,
		"transaction_id": transaction.ID,
		"recipient_id":   recipientID,
	}
	eventData, _ := json.Marshal(event)
	utils.PublishEvent(context.Background(), fmt.Sprintf("%d", userID), eventData)

	// Notify Wallet Sync
	walletURL := os.Getenv("WALLET_SERVICE_URL")
	if walletURL == "" {
		walletURL = "http://wallet-service.railway.internal:8080"
	}

	targetUserID := userID
	if recipientID > 0 {
		targetUserID = recipientID
	}

	addStarsReq := map[string]interface{}{
		"user_id":                targetUserID,
		"star_type":              pkg.Type,
		"amount":                 pkg.Amount,
		"payment_transaction_id": transaction.ID,
		"donor_id":               userID,
	}

	reqBody, _ := json.Marshal(addStarsReq)
	http.Post(walletURL+"/internal/wallet/add-stars", "application/json", bytes.NewBuffer(reqBody))
}

// CreateMercadoPagoPayout initiates a Disbursement/Payout via MercadoPago
func CreateMercadoPagoPayout(c *fiber.Ctx) error {
	var req struct {
		UserID      uint    `json:"user_id"`
		Amount      float64 `json:"amount"`
		AccountInfo string  `json:"account_info"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	accessToken := os.Getenv("MERCADOPAGO_ACCESS_TOKEN")
	if accessToken == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "MercadoPago not configured"})
	}

	// Simple payout logic: disbursement to email or account
	payoutEmail := "test_user_79224068@testuser.com" // Testing default
	if strings.Contains(req.AccountInfo, "@") {
		if strings.Contains(req.AccountInfo, ": ") {
			parts := strings.Split(req.AccountInfo, ": ")
			if len(parts) > 1 && strings.Contains(parts[1], "@") {
				payoutEmail = parts[1]
			}
		} else {
			payoutEmail = req.AccountInfo
		}
	}

	payoutData := map[string]interface{}{
		"amount": req.Amount,
		"collector": map[string]interface{}{
			"email": payoutEmail,
		},
		"payout_method": "account_money",
		"external_reference": fmt.Sprintf("withdrawal-%d-%d", req.UserID, int(req.Amount)),
	}

	jsonData, _ := json.Marshal(payoutData)
	client := &http.Client{}
	mpReq, _ := http.NewRequest("POST", "https://api.mercadopago.com/v1/payouts", bytes.NewBuffer(jsonData))
	mpReq.Header.Set("Authorization", "Bearer "+accessToken)
	mpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(mpReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed down call to MP", "details": err.Error()})
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode >= 400 {
		return c.Status(resp.StatusCode).JSON(result)
	}

	return c.JSON(fiber.Map{
		"message": "Withdrawal processed successfully via MercadoPago",
		"payout":  result,
	})
}
