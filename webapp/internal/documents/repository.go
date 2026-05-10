package documents

import (
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(doc *Document) error {
	return r.db.Create(doc).Error
}

func (r *Repository) ListByUser(userID string) ([]Document, error) {
	var docs []Document
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&docs).Error
	return docs, err
}

func (r *Repository) GetByID(id, userID string) (*Document, error) {
	var doc Document
	err := r.db.Preload("Pages.Blocks").
		Where("id = ? AND user_id = ?", id, userID).
		First(&doc).Error
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *Repository) UpdateStatus(id string, status DocumentStatus, errMsg *string) error {
	updates := map[string]any{"status": status}
	if errMsg != nil {
		updates["error_message"] = errMsg
	}
	return r.db.Model(&Document{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) Delete(id, userID string) error {
	return r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Document{}).Error
}

func (r *Repository) AddPage(page *Page) error {
	return r.db.Create(page).Error
}

func (r *Repository) UpdatePageCount(docID string, count int) error {
	return r.db.Model(&Document{}).Where("id = ?", docID).Update("page_count", count).Error
}
