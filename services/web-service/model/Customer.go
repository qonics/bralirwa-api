package model

import "time"

type Customer struct {
	Id              int       `json:"id"`
	Names           string    `json:"names"`
	MOMONames       *string   `json:"momo_names"`
	Phone           string    `json:"phone"`
	NetworkOperator string    `json:"network_operator"`
	Locale          string    `json:"locale"`
	Province        Province  `json:"province"`
	District        District  `json:"district"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"-"`
}
