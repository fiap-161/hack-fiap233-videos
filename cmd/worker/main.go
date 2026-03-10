// Worker consome a fila video.process, processa cada vídeo (frames → ZIP) e atualiza o status no banco.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/hack-fiap233/videos/internal/adapter/driven/notifier"
	"github.com/hack-fiap233/videos/internal/adapter/driven/postgres"
	"github.com/hack-fiap233/videos/internal/adapter/driven/processor"
	"github.com/hack-fiap233/videos/internal/adapter/driven/queue"
	"github.com/hack-fiap233/videos/internal/adapter/driven/storage"
	"github.com/hack-fiap233/videos/internal/application"
	_ "github.com/lib/pq"
)

func main() {
	amqpURL := os.Getenv("AMQP_URL")
	if amqpURL == "" {
		log.Fatal("AMQP_URL is required")
	}
	queueName := getEnv("QUEUE_NAME", "video.process")
	dlqName := getEnv("QUEUE_DLQ", "video.process.dlq")

	db := initDB()
	videoRepo := postgres.NewVideoRepository(db)
	storageBase := getEnv("STORAGE_BASE_PATH", "./data")
	st, err := storage.NewFilesystemStorage(storageBase)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	proc := processor.NewFFmpegProcessor("")
	noopNotifier := notifier.NewNoopNotifier()

	svc := application.NewVideoService(videoRepo, st, application.WithNotifier(noopNotifier))

	q, err := queue.NewRabbitMQQueue(amqpURL, queueName, dlqName)
	if err != nil {
		log.Fatalf("queue: %v", err)
	}
	defer q.Close()
	deliveries, err := q.Consume(1)
	if err != nil {
		log.Fatalf("consume: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("worker started, consuming %s (prefetch=1)", q.QueueName())
	for {
		select {
		case <-ctx.Done():
			log.Println("worker shutting down")
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			handleDelivery(ctx, d, svc, proc)
		}
	}
}

func handleDelivery(ctx context.Context, d amqp.Delivery, svc *application.VideoService, proc *processor.FFmpegProcessor) {
	var payload queue.VideoJobPayload
	if err := json.Unmarshal(d.Body, &payload); err != nil {
		log.Printf("invalid payload: %v", err)
		_ = d.Nack(false, false)
		return
	}
	err := svc.ProcessJob(ctx, payload.VideoID, payload.UserEmail, proc)
	if err != nil {
		if err == application.ErrInvalidStatus {
			_ = d.Ack(false)
			return
		}
		log.Printf("process job video_id=%d: %v", payload.VideoID, err)
		_ = d.Nack(false, true)
		return
	}
	_ = d.Ack(false)
	log.Printf("processed video_id=%d", payload.VideoID)
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
		log.Fatalf("db open: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}
	return db
}
