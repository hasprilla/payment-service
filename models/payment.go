package models

import (
	"time"

	"gorm.io/gorm"
)

type StarPackage struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"not null" json:"name"`       // e.g. "Silver Stars", "Bronze Stars", "Gold Stars"
	Type      string         `gorm:"uniqueIndex;not null" json:"type"` // e.g. "silver", "bronze", "gold"
	Price     float64        `gorm:"type:decimal(10,2);not null" json:"price"`
	Currency  string         `gorm:"default:'COP'" json:"currency"`
	Amount    int            `gorm:"not null" json:"amount"`     // Number of stars user gets
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type PaymentTransaction struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	UserID        uint           `gorm:"index;not null" json:"user_id"`
	StarPackageID uint           `gorm:"not null" json:"star_package_id"`
	Amount        float64        `gorm:"type:decimal(10,2);not null" json:"amount"`
	Currency      string         `gorm:"default:'COP'" json:"currency"`
	Status        string         `gorm:"default:'completed'" json:"status"` // pending, completed, failed
	PaymentMethod string         `json:"payment_method"`
	RecipientArtistID uint       `gorm:"index" json:"recipient_artist_id,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

type PurchaseRequest struct {
	StarPackageID uint   `json:"star_package_id"`
	CardNumber    string `json:"card_number"`
	Expiry        string `json:"expiry"`
	CVV           string `json:"cvv"`
	Cardholder    string `json:"cardholder"`
	RecipientArtistID uint `json:"recipient_artist_id,omitempty"`
}
