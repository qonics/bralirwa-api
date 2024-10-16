package model

import "time"

type USSDData struct {
	Id           string
	StepId       string
	CustomerId   *int
	CustomerName string
	MSISDN       string
	LastInput    string
	LastResponse string
	NextMenu     string
	NextStepId   string
	NextData     string
	IsCompleted  bool
	// Extra        map[string]interface{}
	// Data         map[string]interface{}
	Language  string
	CreatedAt time.Time
}
