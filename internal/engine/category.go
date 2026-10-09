package engine

import (
	"path/filepath"
	"strings"
)

var categoryByExt = map[string]string{}

func init() {
	for cat, exts := range map[string]string{
		"Video":      "mp4 mkv avi mov wmv flv webm m4v mpg mpeg ts",
		"Music":      "mp3 flac wav aac ogg m4a wma opus",
		"Images":     "jpg jpeg png gif webp bmp svg heic tiff",
		"Documents":  "pdf doc docx xls xlsx ppt pptx txt epub rtf csv odt",
		"Compressed": "zip rar 7z tar gz bz2 xz tgz zst",
		"Programs":   "exe msi dmg pkg deb rpm apk appimage iso",
	} {
		for _, e := range strings.Fields(exts) {
			categoryByExt[e] = cat
		}
	}
}

// CategoryFor maps a file name to a category name, used for folder sorting.
func CategoryFor(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if c, ok := categoryByExt[ext]; ok {
		return c
	}
	return "General"
}
