package notifier

import (
	"context"
	"log"

	"github.com/hack-fiap233/videos/internal/application"
)

// NoopNotifier não envia notificação; só loga. Em produção trocar por adapter SNS - todo
var _ application.FailureNotifier = (*NoopNotifier)(nil)

type NoopNotifier struct{}

func NewNoopNotifier() *NoopNotifier {
	return &NoopNotifier{}
}

func (n *NoopNotifier) NotifyProcessingFailed(ctx context.Context, userID int, userEmail string, videoID int, errorMessage string) error {
	log.Printf("[notifier] processing failed: user_id=%d user_email=%s video_id=%d error=%s (noop, SNS not configured)",
		userID, userEmail, videoID, errorMessage)
	return nil
}
