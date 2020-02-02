package main

// ValidFQDN returns whether str is a valid RFC1035§2.3.1 Fully Qualified Domain Name.
func ValidFQDN(str string) bool {
	// TODO move to lib/go-rfc
	newLabel := true
	prevCh := 'a' // arbitrary previous char which is valid to begin a label.
	for _, ch := range str {
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9' && !newLabel) || // labels cannot begin with numbers
			(ch == '-' && !newLabel) || // labels cannot begin with hyphens
			(ch == '.' && prevCh != '-') { // labels cannot end with hyphens
			prevCh = '-'
			newLabel = false
			continue
		}
		return false
	}
	if prevCh == '-' {
		return false // labels cannot end with hyphens
	}
	return true
}
