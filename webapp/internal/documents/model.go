package documents

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type DocumentStatus string

const (
	StatusPending    DocumentStatus = "pending"
	StatusProcessing DocumentStatus = "processing"
	StatusReady      DocumentStatus = "ready"
	StatusError      DocumentStatus = "error"
)

type Document struct {
	ID               string         `gorm:"type:uuid;primaryKey" json:"id"`
	UserID           string         `gorm:"type:uuid;index;not null" json:"user_id"`
	Title            string         `gorm:"not null" json:"title"`
	OriginalFilename string         `gorm:"not null" json:"original_filename"`
	Status           DocumentStatus `gorm:"default:'pending'" json:"status"`
	ErrorMessage     *string        `json:"error_message,omitempty"`
	OcrSettings      datatypes.JSON `json:"ocr_settings"`
	PageCount        int            `gorm:"default:0" json:"page_count"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`

	Pages []Page `gorm:"foreignKey:DocumentID;constraint:OnDelete:CASCADE" json:"pages,omitempty"`
}

func (d *Document) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	return nil
}

type Page struct {
	ID          string  `gorm:"type:uuid;primaryKey" json:"id"`
	DocumentID  string  `gorm:"type:uuid;index;not null" json:"document_id"`
	PageNumber  int     `gorm:"not null" json:"page_number"`
	RawMarkdown *string `gorm:"type:text" json:"raw_markdown,omitempty"`

	Blocks []Block `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE" json:"blocks,omitempty"`
}

func (p *Page) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	return nil
}

type Block struct {
	ID     string         `gorm:"type:uuid;primaryKey" json:"id"`
	PageID string         `gorm:"type:uuid;index;not null" json:"page_id"`
	Type   string         `gorm:"not null" json:"type"`
	Data   datatypes.JSON `gorm:"not null" json:"data"`
	Order  int            `gorm:"not null;default:0" json:"order"`
}

func (b *Block) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	return nil
}
