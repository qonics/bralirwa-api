package model

import "time"

type PrizeCategory struct {
	Id        int       `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"-"`
}

type PrizeType struct {
	Id            int           `json:"id"`
	Name          string        `json:"name"`
	PrizeCategory PrizeCategory `json:"prize_category"`
	Value         int           `json:"value"`
	Elligibility  int           `json:"elligibility"`
	Status        string        `json:"status"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"-"`
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
