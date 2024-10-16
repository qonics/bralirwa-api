package model

type USSDStep struct {
	Id           string
	Content      string
	Inputs       []USSDInput
	AllowBack    bool
	Validation   *string
	IsEndSession bool
}
