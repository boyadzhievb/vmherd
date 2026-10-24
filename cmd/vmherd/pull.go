package main

import (
	"fmt"

	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <distro:codename>",
		Short: "Download and verify a cloud image",
		Long: `Download a cloud image from the distro vendor and verify its SHA256 checksum.

Examples:
  vmherd pull ubuntu:noble
  vmherd pull debian:bookworm
  vmherd pull fedora:41`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := image.Resolve(args[0])
			if err != nil {
				return err
			}

			cachedPath, err := image.Download(entry)
			if err != nil {
				return err
			}

			fmt.Printf("Image cached at %s\n", cachedPath)
			return nil
		},
	}
}
