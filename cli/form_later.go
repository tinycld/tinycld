package main

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
)

func newFormPublishCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "publish", Hidden: true}
}
func newFormUnpublishCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "unpublish", Hidden: true}
}
func newFormEmbedCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "embed", Hidden: true}
}
func newFormRegenerateCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "regenerate-link", Hidden: true}
}
func newFormSubmissionCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "submission", Hidden: true}
}
func newFormDestinationCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "destination", Hidden: true}
}
func publicFormURL(origin, token string) string { return origin + "/p/forms/" + token }
