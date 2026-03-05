package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	httpadapter "github.com/hack-fiap233/videos/internal/adapter/driver/http"
	"github.com/hack-fiap233/videos/internal/adapter/driven/postgres"
	"github.com/hack-fiap233/videos/internal/application"
	_ "github.com/lib/pq"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	db := initDB()
	if err := postgres.CreateTableIfNotExists(db); err != nil {
		log.Fatalf("Create table: %v", err)
	}

	// Wiring: ports ← adapters (hexagonal)
	videoRepo := postgres.NewVideoRepository(db)
	videoSvc := application.NewVideoService(videoRepo)
	handler := httpadapter.NewVideoHandler(videoSvc, videoRepo) // repo implementa HealthChecker

	// Rotas: health público; demais exigem X-User-Id (API Gateway)
	mux := http.NewServeMux()
	mux.HandleFunc("/videos/health", handler.Health)
	mux.HandleFunc("/videos/", httpadapter.RequireUserID(handler.Videos))

	log.Printf("Videos service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func initDB() *sql.DB {
	sslmode := os.Getenv("DB_SSLMODE")
	if sslmode == "" {
		sslmode = "require"
	}
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USERNAME"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		sslmode,
	)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL")
	return db
}
