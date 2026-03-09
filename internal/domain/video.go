package domain

type Video struct {
	ID            int
	UserID        int
	Title         string
	Description   string
	Status        string
	StorageKey    string
	ResultZipPath string
	ErrorMessage  string
	CreatedAt     string
	UpdatedAt     string
}

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)
