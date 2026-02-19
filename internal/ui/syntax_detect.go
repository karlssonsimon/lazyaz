package ui

import "strings"

func DetectLanguage(blobName, contentType string) string {
	lowerName := strings.ToLower(blobName)
	lowerType := strings.ToLower(contentType)

	switch {
	case strings.HasSuffix(lowerName, ".json") || strings.Contains(lowerType, "json"):
		return "json"
	case strings.HasSuffix(lowerName, ".xml") || strings.Contains(lowerType, "xml"):
		return "xml"
	case strings.HasSuffix(lowerName, ".csv") || strings.Contains(lowerType, "csv"):
		return "csv"
	default:
		return "text"
	}
}

func IsProbablyBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if strings.Contains(string(data), "\x00") {
		return true
	}
	control := 0
	for _, b := range data {
		if b < 0x09 {
			control++
			continue
		}
		if b > 0x0D && b < 0x20 {
			control++
		}
	}
	return float64(control)/float64(len(data)) > 0.05
}
