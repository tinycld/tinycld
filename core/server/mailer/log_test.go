package mailer

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// log_test.go covers the mail capture log: when it is on, and what a full send
// writes to it.

func keepDevAutoLog(t *testing.T) {
	t.Helper()
	saved := devAutoLog
	t.Cleanup(func() { devAutoLog = saved })
}

func TestCaptureLogOn(t *testing.T) {
	keepDevAutoLog(t)

	devAutoLog = false
	t.Setenv("TINYCLD_EMAIL_LOG", filepath.Join(t.TempDir(), "emails.jsonl"))
	if !CaptureLogOn() {
		t.Fatal("CaptureLogOn() = false with TINYCLD_EMAIL_LOG set")
	}

	// A --dev server logs every email.
	t.Setenv("TINYCLD_EMAIL_LOG", "")
	devAutoLog = true
	if !CaptureLogOn() {
		t.Fatal("CaptureLogOn() = false under --dev")
	}

	devAutoLog = false
	if CaptureLogOn() {
		t.Fatal("CaptureLogOn() = true with no log file and no --dev")
	}
}

func TestLogSenderSendFull_LogsReplyToAndAttachmentNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emails.jsonl")
	t.Setenv("TINYCLD_EMAIL_LOG", path)

	_, err := (&LogSender{}).SendFull(context.Background(), &SendRequest{
		From:    `"Ann via Widgets" <noreply@widgets.test>`,
		To:      []Recipient{{Email: "bob@widgets.test"}},
		Subject: "Costs",
		ReplyTo: "ann@widgets.test",
		Attachments: []Attachment{{
			Name: "feeding-costs-2026-11-16.csv", ContentType: "text/csv", Content: "YSxi",
		}},
	})
	if err != nil {
		t.Fatalf("SendFull: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("email log is empty")
	}
	var entry struct {
		ReplyTo         string   `json:"reply_to"`
		Attachments     int      `json:"attachments"`
		AttachmentNames []string `json:"attachment_names"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.ReplyTo != "ann@widgets.test" {
		t.Fatalf("reply_to = %q, want ann@widgets.test\n%s", entry.ReplyTo, scanner.Text())
	}
	if entry.Attachments != 1 || !slices.Equal(entry.AttachmentNames, []string{"feeding-costs-2026-11-16.csv"}) {
		t.Fatalf("attachments %d named %q, want the one file's name\n%s", entry.Attachments, entry.AttachmentNames, scanner.Text())
	}
}
