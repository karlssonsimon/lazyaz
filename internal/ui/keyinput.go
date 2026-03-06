package ui

func ShouldQuit(key string, quit KeyMatcher, filterActive bool) bool {
	if key == "ctrl+c" {
		return true
	}
	if filterActive {
		return false
	}
	return quit.Matches(key)
}
