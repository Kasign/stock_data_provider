package contextassembly

import "unicode/utf8"

func validUTF8(value string) bool {
	return utf8.ValidString(value)
}

func validUTF8Strings(values []string) bool {
	for _, value := range values {
		if !validUTF8(value) {
			return false
		}
	}
	return true
}
