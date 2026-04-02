package ui

import "strings"

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
