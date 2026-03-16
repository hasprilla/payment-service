package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"

	"github.com/harveyasprilla/sonifoy/payment-service/config"
	"github.com/harveyasprilla/sonifoy/payment-service/controllers"
	"github.com/harveyasprilla/sonifoy/payment-service/middleware"
	"github.com/harveyasprilla/sonifoy/payment-service/models"
)

func init() {
	godotenv.Load()
}

func main() {
	config.ConnectDB()
	config.ConnectRedis()

	err := config.DB.AutoMigrate(&models.StarPackage{}, &models.PaymentTransaction{})
	if err != nil {
		log.Fatal("Failed to migrate database: ", err)
	}

	app := fiber.New(fiber.Config{
		AppName: "Sonifoy Payment Service",
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(middleware.Decrypt())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "UP",
			"service": "payment-service",
		})
	})

	api := app.Group("/api/v1/payments")

	// Endpoints
	api.Get("/packages", controllers.GetPackages)
	api.Post("/buy", middleware.Protected(), controllers.BuyStars)

	// Seed star packages if they don't exist
	controllers.SeedPackages(config.DB)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Fatal(app.Listen(":" + port))
}
