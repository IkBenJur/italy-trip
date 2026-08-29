package storage

import (
	"fmt"
	"path"
	"strings"
)

// ContentDisposition builds an attachment header, sanitising the filename so a
// stray quote or slash cannot break out of the header value.
func ContentDisposition(filename string) string {
	clean := path.Base(strings.TrimSpace(filename))
	clean = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\\' || r == '/' || r == 0x7f {
			return '-'
		}
		return r
	}, clean)

	if clean == "" || clean == "." || clean == ".." {
		clean = "photo.jpg"
	}

	return fmt.Sprintf("attachment; filename=%q", clean)
}
