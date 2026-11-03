package main

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
)

func newFormDestinationCmd(*client.Client) *cobra.Command {
	return &cobra.Command{Use: "destination", Hidden: true}
}
