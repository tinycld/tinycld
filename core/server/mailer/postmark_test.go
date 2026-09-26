package mailer

import "testing"

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
