package model

import "time"

type UserProfile struct {
	Id             int        `json:"id"`
	Email          string     `json:"email"`
	Fname          string     `json:"firstname"`
	Lname          string     `json:"lastname"`
	Phone          string     `json:"phone"`
	Department     Department `json:"department"`
	CanAddCodes    bool       `json:"can_add_codes"`
	CanTriggerDraw bool       `json:"can_trigger_draw"`
	CanAddUser     bool       `json:"can_add_user"`
	CanViewLogs    bool       `json:"can_view_logs"`
	EmailVerified  bool       `json:"email_verified"`
	PhoneVerified  bool       `json:"phone_verified"`
	AvatarUrl      string     `json:"avatar_url"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	AccessToken    string     `json:"-"`
}
