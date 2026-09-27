package mailer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mrz1836/postmark"
)

func TestBuildPostmarkEmail_CopiesMetadata(t *testing.T) {
	req := &SendRequest{From: "a@widgets.test", Metadata: map[string]string{"sender_domain": "widgets.test"}}
	email := buildPostmarkEmail(req, "")
	if email.Metadata["sender_domain"] != "widgets.test" {
		t.Fatalf("Metadata = %v, want sender_domain=widgets.test", email.Metadata)
	}
}

func TestBuildPostmarkEmail_NoMetadataStaysNil(t *testing.T) {
	email := buildPostmarkEmail(&SendRequest{From: "a@widgets.test"}, "")
	if email.Metadata != nil {
		t.Fatalf("Metadata = %v, want nil so the JSON omits it", email.Metadata)
	}
}

func TestPostmarkSender_APIURLOverrideReceivesTheSend(t *testing.T) {
	var gotPath, gotToken string
	var gotEmail postmark.Email
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Postmark-Server-Token")
		_ = json.NewDecoder(r.Body).Decode(&gotEmail)
		_ = json.NewEncoder(w).Encode(postmark.EmailResponse{MessageID: "stand-in-1"})
	}))
	t.Cleanup(srv.Close)
	withConfig(t, map[string]string{keyPostmarkAPIURL: srv.URL + "/"})

	s := NewPostmarkSender("server-tok", "", "noreply@widgets.test")
	res, err := s.SendFull(context.Background(), &SendRequest{
		To:      []Recipient{{Email: "a@widgets.test"}},
		Subject: "hi",
	})
	if err != nil {
		t.Fatalf("SendFull: %v", err)
	}
	if res.ProviderMessageID != "stand-in-1" {
		t.Errorf("ProviderMessageID = %q, want stand-in-1", res.ProviderMessageID)
	}
	if gotPath != "/email" || gotToken != "server-tok" || gotEmail.Subject != "hi" {
		t.Errorf("request = %s token=%q subject=%q, want /email server-tok hi", gotPath, gotToken, gotEmail.Subject)
	}
}

func TestPostmarkSender_UnsetAPIURLKeepsPostmark(t *testing.T) {
	withConfig(t, map[string]string{})
	if got := NewPostmarkSender("tok", "", "").Client().BaseURL; got != postmark.NewClient("", "").BaseURL {
		t.Errorf("BaseURL = %q, want the client's default", got)
	}
}
