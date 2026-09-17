package koi

import (
	"mime"
	"strconv"
	"strings"
)

func mediaType(format string) string {
	switch format {
	case "html":
		return "text/html"
	case "json":
		return "application/json"
	}
	return format
}

// More specific ranges override wildcards, including explicit exclusions.
func quality(accept, target string) float64 {
	if strings.TrimSpace(accept) == "" {
		return 1
	}
	best, q := -1, float64(0)
	for item := range strings.SplitSeq(accept, ",") {
		kind, params, err := mime.ParseMediaType(strings.TrimSpace(item))
		if err != nil {
			continue
		}
		specificity := -1
		switch {
		case kind == target:
			specificity = 2
		case kind == strings.SplitN(target, "/", 2)[0]+"/*":
			specificity = 1
		case kind == "*/*":
			specificity = 0
		}
		if specificity < 0 {
			// An unrelated media type does not make the target acceptable.
			continue
		}
		weight := float64(1)
		if value, ok := params["q"]; ok {
			weight, err = strconv.ParseFloat(value, 64)
			if err != nil || !(weight >= 0 && weight <= 1) {
				continue
			}
		}
		if specificity > best || specificity == best && weight > q {
			best, q = specificity, weight
		}
	}
	return q
}
