package model

import "time"

type Province struct {
	Id        int       `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

type District struct {
	Id        int       `json:"id"`
	Name      string    `json:"name"`
	Province  Province  `json:"-"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}
