package util

func CutStr(s string, maxSize int) string {
	if len(s) <= maxSize {
		return s
	}
	return s[:maxSize-3] + "..."
}
