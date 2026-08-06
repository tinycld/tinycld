package p

type Base struct {
	ID string `json:"id"`
}

type Broken struct {
	Base
	Name string `json:"name"`
}
