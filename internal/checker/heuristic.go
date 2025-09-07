package checker

import (
	"bytes"
	"regexp"
)

var rules = []*regexp.Regexp{
	// redirects are bad
	regexp.MustCompile(`<meta\s+http-equiv\s*=\s*["']refresh["']\s+`),
	// captcha is same as redirect but with extra steps
	regexp.MustCompile(`<title>\s*вы не робот\?\s*<\/title>`),
}

func containsSpamContent(data []byte) bool {
	data = bytes.ToLower(data)
	for _, rule := range rules {
		if rule.Match(data) {
			return true
		}
	}
	return false
}
