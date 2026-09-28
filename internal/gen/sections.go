package main

import "fmt"

// sectionFiles names the file of package models for each section of the Bot API page.
var sectionFiles = map[string]string{
	"Getting updates":   "updates",
	"Available types":   "available",
	"Stickers":          "stickers",
	"Inline mode":       "inline",
	"Payments":          "payments",
	"Telegram Passport": "passport",
	"Games":             "games",
	"Rich messages":     "rich",
}

// sectionFile returns the file of package models that holds the types of a section.
func sectionFile(section string) (string, error) {
	name, ok := sectionFiles[section]
	if !ok {
		return "", fmt.Errorf("the section %q of the Bot API page has no file: add it to sectionFiles", section)
	}
	return "models/" + name + ".gen.go", nil
}
