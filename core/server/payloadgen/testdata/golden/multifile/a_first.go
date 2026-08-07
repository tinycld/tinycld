package multi

// Reply references a type from a sibling file.
type Reply struct {
	Ack Ack `json:"ack"`
}
