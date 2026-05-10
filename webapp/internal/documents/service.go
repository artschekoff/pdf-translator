package documents

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yourorg/pdf-translator-webapp/config"
	"github.com/yourorg/pdf-translator-webapp/internal/shared"
	"gorm.io/datatypes"
)

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) Upload(userID, filename string, r io.Reader, settings shared.OcrSettings) (*Document, error) {
	settingsJSON, _ := json.Marshal(settings)

	doc := &Document{
		UserID:           userID,
		Title:            filename[:len(filename)-len(filepath.Ext(filename))],
		OriginalFilename: filename,
		OcrSettings:      datatypes.JSON(settingsJSON),
		Status:           StatusPending,
	}
	if err := s.repo.Create(doc); err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	dest := filepath.Join(config.C.UploadsDir, doc.ID+".pdf")
	f, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *Service) List(userID string) ([]Document, error) { return s.repo.ListByUser(userID) }
func (s *Service) Get(id, userID string) (*Document, error) { return s.repo.GetByID(id, userID) }

func (s *Service) Delete(id, userID string) error {
	if err := s.repo.Delete(id, userID); err != nil {
		return err
	}
	os.Remove(filepath.Join(config.C.UploadsDir, id+".pdf"))
	return nil
}
