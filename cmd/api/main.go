package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"ton-tracer/api/handlers"
	"ton-tracer/pkg/database"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "ton-tracer/docs" // Import generated docs
)

// @title TON Transaction Tracer API with Database Cache
// @version 1.1.0
// @description API for tracing TON blockchain transactions with PostgreSQL caching for improved performance

// @contact.name API Support
// @contact.email support@example.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host tracer.zaruchevskiy.ru
// @BasePath /
func main() {
	port := flag.String("port", "8080", "Server port")
	testnet := flag.Bool("testnet", false, "Use testnet")
	flag.Parse()

	// API configuration from environment variables
	apiHost := getEnv("API_HOST", "tracer.zaruchevskiy.ru")

	// Database configuration from environment variables
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnvAsInt("DB_PORT", 5432)
	dbUser := getEnv("DB_USER", "tontracer")
	dbPassword := getEnv("DB_PASSWORD", "tontracer")
	dbName := getEnv("DB_NAME", "tontracer")
	dbSSLMode := getEnv("DB_SSLMODE", "disable")

	// Store API host for potential use
	_ = apiHost

	// Initialize database
	var repo *database.Repository
	dbConfig := database.Config{
		Host:     dbHost,
		Port:     dbPort,
		User:     dbUser,
		Password: dbPassword,
		DBName:   dbName,
		SSLMode:  dbSSLMode,
	}

	db, err := database.NewDB(dbConfig)
	if err != nil {
		log.Printf("Warning: Failed to connect to database: %v", err)
		log.Println("Running without database cache")
		repo = nil
	} else {
		// Initialize schema
		if err := db.InitSchema(); err != nil {
			log.Printf("Warning: Failed to initialize database schema: %v", err)
			log.Println("Running without database cache")
			repo = nil
		} else {
			repo = database.NewRepository(db)
			log.Println("Database cache enabled")
		}
	}

	// Initialize handler with repository
	handler, err := handlers.NewHandler(*testnet, repo)
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

		// Cache management
		v1.GET("/cache/stats", handler.GetCacheStats)
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

// Helper functions for environment variables
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
