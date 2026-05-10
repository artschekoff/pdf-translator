package billing

import "time"

type Plan struct {
	ID               string    `gorm:"type:uuid;primaryKey" json:"id"`
	Name             string    `json:"name"`
	Pages            int       `json:"pages"`
	PriceCents       int       `json:"price_cents"`
	PaymentsPriceID  *string   `gorm:"type:uuid" json:"payments_price_id,omitempty"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
}

func (Plan) TableName() string { return "billing_plans" }

type UserBalance struct {
	UserID    string    `gorm:"primaryKey" json:"user_id"`
	Pages     int       `json:"pages"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (UserBalance) TableName() string { return "user_balances" }

type CreditedPurchase struct {
	PurchaseID  string    `gorm:"primaryKey" json:"purchase_id"`
	UserID      string    `json:"user_id"`
	Pages       int       `json:"pages"`
	CreditedAt  time.Time `json:"credited_at"`
}

func (CreditedPurchase) TableName() string { return "credited_purchases" }
