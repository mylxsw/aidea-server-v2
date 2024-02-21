package misc

import "regexp"

const emailRegex = `^([\w\.\_\-]{2,30})@(\w{1,}).([a-z]{2,8})$`
const phoneRegex = `^1[3456789]\d{9}$`

// IsEmail check if the value is a valid email
func IsEmail(value string) bool {
	return regexp.MustCompile(emailRegex).MatchString(value)
}

// IsPhoneNumber check if the value is a valid phone number
func IsPhoneNumber(value string) bool {
	return regexp.MustCompile(phoneRegex).MatchString(value)
}
