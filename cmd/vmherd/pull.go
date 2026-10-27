package main

import (
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/spf13/cobra"
)

// newPullCmd creates the 'pull' command that downloads cloud images.
func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <distro:codename>",
		Short: "Download and verify a cloud image",
		Long: `Download a cloud image and verify its checksum.
The image URL is resolved from the Herdfile images section (or defaults if no Herdfile).

Examples:
  vmherd pull ubuntu:noble
  vmherd pull ubuntu:resolute
  vmherd pull fedora:42`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			images := image.DefaultImages()

			project, loadError := config.LoadProject()
			if loadError == nil && len(project.Herdfile.Images) > 0 {
				images = project.Herdfile.Images
			}

			entry, err := image.Resolve(args[0], images)
			if err != nil {
				return err
			}

			cachedPath, err := image.Download(entry)
			if err != nil {
				return err
			}

			ui.Printf("Image cached at %s\n", cachedPath)
			return nil
		},
	}
}
