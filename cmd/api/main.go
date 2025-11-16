package main

import (
	"flag"
	"log"
	"ton-tracer/api/handlers"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "ton-tracer/docs" // Import generated docs
)

// @title TON Transaction Tracer API
// @version 1.0
// @description API for tracing TON blockchain transactions and analyzing balance flows

// @contact.name API Support
// @contact.email support@example.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host desktop.lan:8080
// @BasePath /
func main() {
	port := flag.String("port", "8080", "Server port")
	testnet := flag.Bool("testnet", false, "Use testnet")
	flag.Parse()

	// Initialize handler
	handler, err := handlers.NewHandler(*testnet)
	if err != nil {
		log.Fatalf("Failed to initialize handler: %v", err)
	}

	// Setup Gin
	router := gin.Default()

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check endpoint
	router.GET("/health", handler.HealthCheck)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Transaction tracing
		v1.POST("/trace", handler.TraceTransaction)
		v1.POST("/transactions/recent", handler.GetRecentTransactions)

		// DNS resolution
		v1.POST("/dns/resolve", handler.ResolveDNS)
	}

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Redirect root to swagger
	router.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/swagger/index.html")
	})

	log.Printf("Starting TON Tracer API on port %s", *port)
	if *testnet {
		log.Println("Using TESTNET")
	}

	if err := router.Run(":" + *port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
