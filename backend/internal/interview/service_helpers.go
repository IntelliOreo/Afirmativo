package interview

import "strings"

func normalizePreferredLanguage(preferredLanguage string) string {
	if strings.EqualFold(strings.TrimSpace(preferredLanguage), "en") {
		return "en"
	}
	return "es"
}

func emptyAnswerPlaceholder(preferredLanguage string) string {
	if normalizePreferredLanguage(preferredLanguage) == "en" {
		return "[No response provided]"
	}
	return "[No se proporciono respuesta]"
}
