package model

type UserProfile struct {
	Id         uint
	Username   string
	Email      string
	Names      string
	Gender     string
	Image      string
	Status     uint
	SmsBalance uint `json:"sms_balance"`
}
