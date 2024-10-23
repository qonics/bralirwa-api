package model

import "time"

type Code struct {
	Id        int        `json:"id"`
	Code      string     `json:"code"`
	PrizeType *PrizeType `json:"prize_type"`
	Redeemed  bool       `json:"redeemed"`
	Status    string     `json:"status,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
