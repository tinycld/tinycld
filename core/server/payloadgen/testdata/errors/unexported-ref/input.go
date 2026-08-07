package p

type inner struct {
	OK bool `json:"ok"`
}

type Broken struct {
	Inner inner `json:"inner"`
}
