package model

import "time"

type Customer struct {
	Id              int       `json:"id"`
	Names           string    `json:"names"`
	MOMONames       *string   `json:"momo_names,omitempty"`
	Phone           string    `json:"phone"`
	NetworkOperator string    `json:"network_operator,omitempty"`
	Locale          string    `json:"locale,omitempty"`
	Province        Province  `json:"province,omitempty"`
	District        District  `json:"district,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"-"`
}
