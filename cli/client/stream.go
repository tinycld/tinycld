package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// PostStream POSTs body (JSON-encoded) to path and hands back the response
// body unread, for endpoints that answer with a long-lived stream (a progress
// feed, an archive download). The caller must close the returned body.
//
// Like Get it uses DoStream: the response may take arbitrarily long to finish,
// so the client's whole-exchange timeout must not apply — bound the call with
// ctx instead. GetBody is set so a mid-flight 401 refresh can replay the
// request whole.
func (c *Client) PostStream(ctx context.Context, path string, body any) (io.ReadCloser, *http.Response, error) {
	if err := c.ensure(); err != nil {
		return nil, nil, err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	resp, err := c.DoStream(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, resp, apiError(resp.StatusCode, errBody)
	}
	return resp.Body, resp, nil
}
