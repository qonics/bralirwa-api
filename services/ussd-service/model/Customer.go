package model

import "time"

type Customer struct {
	Id              int
	Names           string
	Phone           string
	NetworkOperator string
	Locale          string
	Province        Province
	District        District
	IdNumber        *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
