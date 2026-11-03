package main

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
)

func newFormSubmissionCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "submission", Hidden: true}
}

func newFormDestinationCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "destination", Hidden: true}
}
