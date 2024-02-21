package misc

// StringDefault returns def if s is empty
func StringDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// BoolDefault returns def if b is false
func BoolDefault(b, def bool) bool {
	if !b {
		return def
	}
	return b
}

// IntDefault returns def if i is 0
func IntDefault(i, def int) int {
	if i == 0 {
		return def
	}

	return i
}
