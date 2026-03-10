package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	httpadapter "github.com/hack-fiap233/videos/internal/adapter/driver/http"
	"github.com/hack-fiap233/videos/internal/adapter/driven/notifier"
	"github.com/hack-fiap233/videos/internal/adapter/driven/postgres"
	"github.com/hack-fiap233/videos/internal/adapter/driven/queue"
	"github.com/hack-fiap233/videos/internal/adapter/driven/storage"
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

	videoRepo := postgres.NewVideoRepository(db)
	storageBase := os.Getenv("STORAGE_BASE_PATH")
	if storageBase == "" {
		storageBase = "./data"
	}
	st, err := storage.NewFilesystemStorage(storageBase)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	var videoQueue application.VideoQueue
	if amqpURL := os.Getenv("AMQP_URL"); amqpURL != "" {
		q, err := queue.NewRabbitMQQueue(amqpURL, getEnv("QUEUE_NAME", "video.process"), getEnv("QUEUE_DLQ", "video.process.dlq"))
		if err != nil {
			log.Printf("queue disabled: %v", err)
		} else {
			videoQueue = q
			defer func() { _ = q.Close() }()
		}
	}

	videoSvc := application.NewVideoService(videoRepo, st,
		application.WithQueue(videoQueue),
		application.WithNotifier(notifier.NewNoopNotifier()),
	)
	handler := httpadapter.NewVideoHandler(videoSvc, videoRepo)

	r := chi.NewRouter()
	r.Route("/videos", func(r chi.Router) {
		r.Get("/health", handler.Health)
		r.Group(func(r chi.Router) {
			r.Use(httpadapter.RequireUserIDHandler)
			r.Post("/upload", handler.Upload)
			r.Get("/", handler.List)
			r.Post("/", handler.Create)
			r.Get("/{id}", handler.GetByID)
			r.Get("/{id}/download", handler.Download)
		})
	})

	log.Printf("Videos service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
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
