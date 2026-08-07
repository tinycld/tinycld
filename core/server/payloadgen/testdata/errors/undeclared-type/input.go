package p

type Broken struct {
	X Missing `json:"x"`
}
