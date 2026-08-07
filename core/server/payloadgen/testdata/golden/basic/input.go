// Package sample exercises every mapping the generator supports.
package sample

import "time"

// MultipartFieldJSON is the multipart form key carrying the JSON body.
const MultipartFieldJSON = "json"

const MaxAttachments = 20

// Recipient is a single name/address pair.
type Recipient struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SendRequest documents the full mapping surface. */ escape check.
type SendRequest struct {
	// To is the primary recipient list.
	To       []Recipient       `json:"to"`
	Cc       []Recipient       `json:"cc,omitempty"`
	Subject  string            `json:"subject"`
	Draft    bool              `json:"draft"`
	Size     int64             `json:"size"`
	Ratio    float64           `json:"ratio"`
	Raw      []byte            `json:"raw"`
	Labels   map[string]string `json:"labels"`
	Details  *Details          `json:"details"`
	Optional *Details          `json:"optional_details,omitempty"`
	Nested   [][]int           `json:"nested"`
	PtrList  []*Details        `json:"ptr_list"`
	SentAt   time.Time         `json:"sent-at"`
	internal string
	Ignored  string `json:"-"`
}

// Details is referenced before its declaration.
type Details struct {
	OK bool `json:"ok"`
}

var _ = internalHelper

// internalHelper proves unexported, unreferenced declarations are skipped.
func internalHelper() string { return SendRequest{internal: ""}.internal }
