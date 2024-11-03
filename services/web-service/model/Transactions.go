package model

import "time"

type Transactions struct {
	Id              int       `json:"id"`
	PrizeId         int       `json:"prize_id"`
	Amount          int       `json:"amount"`
	Phone           string    `json:"phone"`
	Mno             string    `json:"mno"`
	TrxId           string    `json:"trx_id"`
	RefNo           string    `json:"ref_no"`
	TransactionType string    `json:"transaction_type"`
	CustomerId      int       `json:"customer_id"`
	InitiatedBy     string    `json:"initiated_by"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
