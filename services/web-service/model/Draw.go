package model

import "time"

type Draw struct {
	Id        int       `json:"id"`
	Code      string    `json:"code"`
	PrizeType PrizeType `json:"prize_type"`
	Customer  Customer  `json:"customer"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
