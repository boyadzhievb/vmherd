package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/spf13/cobra"
)

// newListImagesCmd creates the 'list-images' command that shows available cloud images.
func newListImagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-images",
		Short: "Show cloud images defined in the Herdfile (or defaults if no Herdfile)",
		RunE: func(cmd *cobra.Command, args []string) error {
			images := image.DefaultImages()

			project, loadError := config.LoadProject()
			if loadError == nil && len(project.Herdfile.Images) > 0 {
				images = project.Herdfile.Images
			}

			entries := image.ListAll(images)

			writer := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(writer, "IMAGE\tDESCRIPTION\tCACHED")
			_, _ = fmt.Fprintln(writer, "-----\t-----------\t------")

			for _, entry := range entries {
				cached := "no"
				if image.IsCached(&entry) {
					cached = "yes"
				}
				_, _ = fmt.Fprintf(writer, "%s:%s\t%s\t%s\n",
					entry.Distro, entry.Codename,
					entry.Description,
					cached,
				)
			}

			_ = writer.Flush()
			return nil
		},
	}
}
