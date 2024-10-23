package model

import "time"

type Entries struct {
	Id        int       `json:"id"`
	Customer  Customer  `json:"customer"`
	Code      Code      `json:"code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"-"`
}
