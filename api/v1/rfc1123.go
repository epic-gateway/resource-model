package v1

import "strings"

var (
	// rfc1123Cleaner "cleans" string-formatted IP addresses so they can
	// be used in RFC1123 hostnames.
	rfc1123Cleaner = strings.NewReplacer(
		".", "-",
		":", "-",
	)
)
