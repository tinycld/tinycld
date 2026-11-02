package webforms

import (
	"crypto/hmac"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	stampMinAge = 3 * time.Second
	stampMaxAge = 2 * time.Hour
)

var (
	errStamp   = errors.New("invalid stamp")
	errTooFast = errors.New("submitted too fast")
)

// issueStamp signs the publication and the time the page got the form. A bot
// that posts without loading the form has no stamp; one that posts at once
// fails the minimum age.
func issueStamp(key []byte, pubID string, now time.Time) string {
	payload := pubID + "." + strconv.FormatInt(now.UnixMilli(), 10)
	return payload + "." + sign(key, payload)
}

func checkStamp(key []byte, stamp, pubID string, now time.Time) error {
	i := strings.LastIndex(stamp, ".")
	if i < 0 {
		return errStamp
	}
	payload, sig := stamp[:i], stamp[i+1:]
	if !hmac.Equal([]byte(sig), []byte(sign(key, payload))) {
		return errStamp
	}
	parts := strings.SplitN(payload, ".", 2)
	if len(parts) != 2 || parts[0] != pubID {
		return errStamp
	}
	ms, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return errStamp
	}
	age := now.Sub(time.UnixMilli(ms))
	if age > stampMaxAge {
		return errStamp
	}
	if age < stampMinAge {
		return errTooFast
	}
	return nil
}
