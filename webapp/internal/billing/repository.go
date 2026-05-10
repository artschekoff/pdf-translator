package billing

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) ListPlans() ([]Plan, error) {
	var plans []Plan
	err := r.db.Where("is_active = true").Order("price_cents asc").Find(&plans).Error
	return plans, err
}

// PlanByPaymentsPriceID finds a plan whose payments_price_id matches.
func (r *Repository) PlanByPaymentsPriceID(priceID string) (*Plan, error) {
	var p Plan
	err := r.db.First(&p, "payments_price_id = ? AND is_active = true", priceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

func (r *Repository) GetBalance(userID string) (int, error) {
	var b UserBalance
	err := r.db.First(&b, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return b.Pages, err
}

func (r *Repository) AddPages(userID string, pages int) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"pages":      gorm.Expr("user_balances.pages + ?", pages),
			"updated_at": time.Now(),
		}),
	}).Create(&UserBalance{UserID: userID, Pages: pages, UpdatedAt: time.Now()}).Error
}

func (r *Repository) DeductPages(userID string, pages int) error {
	return r.db.Exec(
		`UPDATE user_balances SET pages = GREATEST(pages - ?, 0), updated_at = NOW() WHERE user_id = ?`,
		pages, userID,
	).Error
}

func (r *Repository) IsPurchaseCredited(purchaseID string) (bool, error) {
	var count int64
	err := r.db.Model(&CreditedPurchase{}).Where("purchase_id = ?", purchaseID).Count(&count).Error
	return count > 0, err
}

func (r *Repository) RecordCreditedPurchase(purchaseID, userID string, pages int) error {
	return r.db.Create(&CreditedPurchase{
		PurchaseID: purchaseID,
		UserID:     userID,
		Pages:      pages,
		CreditedAt: time.Now(),
	}).Error
}
