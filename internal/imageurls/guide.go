// Package imageurls checks the image URLs vmherd ships and documents before a
// release, and rewrites stale ones in the working tree for the maintainer to
// review. It is used by the developer tool cmd/image-urls-check, not by vmherd.
package imageurls

import (
	"fmt"
	"strings"
)

// GuideRelativePath is the image URL guide, relative to the repository root;
// its table lists every URL to check.
const GuideRelativePath = "docs/images.md"

// guideTableHeading is the heading the URL table sits under in the guide.
const guideTableHeading = "## Booted URLs"

// guideTableHeaderAndSeparatorRows are the table's header and |---| rows,
// which hold no image.
const guideTableHeaderAndSeparatorRows = 2

// GuideImage is one row of the guide's URL table.
type GuideImage struct {
	Name string // image name in distro:codename form, e.g. fedora:45
	URL  string // vendor download URL (https)
}

// ParseGuideTable returns the rows of the | Image | URL | Notes | table under
// '## Booted URLs' in the image guide's content, in table order.
func ParseGuideTable(guideContent string) ([]GuideImage, error) {
	tableRows := guideTableRows(guideContent)
	if len(tableRows) <= guideTableHeaderAndSeparatorRows {
		return nil, fmt.Errorf("no image table under '%s' in %s: add the table | Image | URL | Notes | with one row per image",
			guideTableHeading, GuideRelativePath)
	}

	var guideImages []GuideImage
	for _, tableRow := range tableRows[guideTableHeaderAndSeparatorRows:] {
		rowCells := strings.Split(strings.Trim(tableRow, "|"), "|")
		if len(rowCells) < 2 {
			return nil, fmt.Errorf("row %q in the '%s' table of %s has no URL column: use | Image | URL | Notes |",
				tableRow, guideTableHeading, GuideRelativePath)
		}
		guideImage := GuideImage{Name: unwrapTableCell(rowCells[0]), URL: unwrapTableCell(rowCells[1])}
		if guideImage.Name == "" || !strings.HasPrefix(guideImage.URL, "https://") {
			return nil, fmt.Errorf("row %q in the '%s' table of %s needs an image name and an https:// URL",
				tableRow, guideTableHeading, GuideRelativePath)
		}
		guideImages = append(guideImages, guideImage)
	}
	return guideImages, nil
}

// guideTableRows returns the consecutive '|' lines of the first table after
// the guide's '## Booted URLs' heading, before the next '## ' heading.
func guideTableRows(guideContent string) []string {
	var tableRows []string
	underTableHeading := false
	for _, guideLine := range strings.Split(guideContent, "\n") {
		trimmedLine := strings.TrimSpace(guideLine)
		switch {
		case trimmedLine == guideTableHeading:
			underTableHeading = true
		case !underTableHeading:
		case strings.HasPrefix(trimmedLine, "## "):
			return tableRows
		case strings.HasPrefix(trimmedLine, "|"):
			tableRows = append(tableRows, trimmedLine)
		case len(tableRows) > 0:
			return tableRows
		}
	}
	return tableRows
}

// unwrapTableCell trims a table cell and the `code` or <autolink> marks
// around its text.
func unwrapTableCell(tableCell string) string {
	cellText := strings.TrimSpace(tableCell)
	cellText = strings.Trim(cellText, "`")
	if strings.HasPrefix(cellText, "<") && strings.HasSuffix(cellText, ">") {
		cellText = cellText[1 : len(cellText)-1]
	}
	return strings.TrimSpace(cellText)
}
