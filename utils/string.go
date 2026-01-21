package utils

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func TitleCase(s string) string {
	titleCaser := cases.Title(language.English)
	return titleCaser.String(s)
}
