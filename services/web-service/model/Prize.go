package model

import "time"

type PrizeCategory struct {
	Id        int       `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"-"`
}
type PrizeMessage struct {
	Lang    string `json:"lang" binding:"required" validate:"required,oneof=en rw"`
	Message string `json:"message" binding:"required" validate:"required,min=10,max=255"`
}
type PrizeType struct {
	Id            int            `json:"id"`
	Name          string         `json:"name"`
	PrizeCategory PrizeCategory  `json:"prize_category"`
	Value         int            `json:"value"`
	Elligibility  int            `json:"elligibility"`
	Period        string         `json:"period"`
	Distribution  string         `json:"distribution"`
	ExpiryDate    time.Time      `json:"expiry_date"`
	PrizeMessage  []PrizeMessage `json:"messages"`
	Status        string         `json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"-"`
}

type Prize struct {
	Id            int           `json:"id"`
	PrizeType     PrizeType     `json:"prize_type"`
	PrizeCategory PrizeCategory `json:"prize_category"`
	Value         int           `json:"value"`
	Code          string        `json:"code"`
	Rewarded      bool          `json:"rewarded"`
	Customer      Customer      `json:"customer"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"-"`
}
