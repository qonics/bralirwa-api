package model

import "time"

type Transactions struct {
	Id              int       `json:"id"`
	PrizeId         int       `json:"prize_id"`
	Amount          int       `json:"amount"`
	Phone           string    `json:"phone"`
	Mno             string    `json:"mno"`
	TrxId           string    `json:"trx_id"`
	RefNo           *string   `json:"ref_no"`
	TransactionType string    `json:"transaction_type"`
	CustomerId      int       `json:"customer_id"`
	InitiatedBy     string    `json:"initiated_by"`
	Code            string    `json:"code"`
	ErrorMessage    *string   `json:"error_message"`
	EntryId         int       `json:"entry_id"`
	Charges         int       `json:"charges"`
	Status          string    `json:"status"`
	PrizeType       string    `json:"prize_type"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
