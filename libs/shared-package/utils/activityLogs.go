package utils

import "time"

type ActivityLog struct {
	UserID       int       `json:"user_id"`
	ActivityType string    `json:"activity_type"`
	Status       string    `json:"status"`
	Description  string    `json:"description"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
	User         UserData  `json:"user"`
}

type UserData struct {
	Id    int    `json:"id,omitempty"`
	Fname string `json:"fname"`
	Lname string `json:"lname"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}
