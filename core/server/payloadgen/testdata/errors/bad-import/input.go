package p

import "net/http"

type Broken struct {
	H http.Header `json:"h"`
}
