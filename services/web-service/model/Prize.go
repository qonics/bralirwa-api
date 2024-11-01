package model

import "time"

type PrizeCategory struct {
	Id        int       `json:"id"`
	Name      string    `json:"name,omitempty"`
	Status    string    `json:"status,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"-"`
}
type PrizeMessage struct {
	Lang    string `json:"lang" binding:"required" validate:"required,oneof=en rw"`
	Message string `json:"message" binding:"required" validate:"required,min=10,max=255"`
}
type PrizeType struct {
	Id            int            `json:"id"`
	Name          string         `json:"name,omitempty"`
	PrizeCategory PrizeCategory  `json:"prize_category,omitempty"`
	Value         int            `json:"value,omitempty"`
	Elligibility  int            `json:"elligibility,omitempty"`
	Period        string         `json:"period,omitempty"`
	Distribution  string         `json:"distribution,omitempty"`
	ExpiryDate    *time.Time     `json:"expiry_date,omitempty"`
	PrizeMessage  []PrizeMessage `json:"messages,omitempty"`
	Status        string         `json:"status,omitempty"`
	CreatedAt     time.Time      `json:"created_at,omitempty"`
	UpdatedAt     time.Time      `json:"-"`
}

type Prize struct {
	Id            int           `json:"id"`
	PrizeType     PrizeType     `json:"prize_type,omitempty"`
	PrizeCategory PrizeCategory `json:"prize_category,omitempty"`
	Value         int           `json:"value,omitempty"`
	Code          string        `json:"code,omitempty"`
	Rewarded      bool          `json:"rewarded,omitempty"`
	Customer      Customer      `json:"customer,omitempty"`
	CreatedAt     time.Time     `json:"created_at,omitempty"`
	UpdatedAt     time.Time     `json:"-"`
}
