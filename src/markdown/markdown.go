package markdown

import (
	"regexp"
	"strings"
)

func EnsureFormatting(text string) string {
	numDelimiters := strings.Count(text, "```")

	if (numDelimiters % 2) == 1 {
		text += "```"
	}

	segs := strings.Split(text, "```")
	for i, seg := range segs {
		if (i % 2) == 0 { // not in code block
			if n := strings.Count(seg, "`"); (n % 2) == 1 {
				segs[i] += "`"
			}
		}
	}
	text = strings.Join(segs, "```")

	// replace * with \* and _ with \_
	re := regexp.MustCompile(`([*_])`)
	text = re.ReplaceAllString(text, `\$1`)

	return text
}
