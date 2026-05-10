package queue

import "time"

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type DocumentJob struct {
	ID         string    `gorm:"primaryKey"`
	InputPath  string    `gorm:"not null"`
	OutputPath string    `gorm:"not null"`
	SourceLang string    `gorm:"not null"`
	TargetLang string    `gorm:"not null"`
	OCREngine  string    `gorm:"not null;default:paddleocr"`
	TotalPages int       `gorm:"not null"`
	TempDir    string    `gorm:"not null"`
	DPI          int       `gorm:"not null;default:300"`
	Password     string    `gorm:"-"`
	KeepOriginal bool      `gorm:"not null;default:false"`
	Status       JobStatus `gorm:"not null;default:pending"`
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PageJob struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	DocumentID  string    `gorm:"index;not null"`
	PageNum     int       `gorm:"not null"`
	PageType    string    `gorm:"not null"` // "native" or "scanned"
	Status      JobStatus `gorm:"not null;default:pending"`
	Retries     int       `gorm:"not null;default:0"`
	Error       string
	OutputFile  string
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
