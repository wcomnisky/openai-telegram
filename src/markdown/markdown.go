package markdown

import (
	"regexp"
	"strings"
)

func EnsureFormatting(text string, block_closed bool) (string, bool) {
	segs := strings.Split(text, "```")

	if !block_closed { // prepend "```" because the previous code block was not closed
		segs = append([]string{""}, segs...)
		block_closed = true
	}

	if (len(segs) % 2) == 0 { // append "```" because the current code block is not closed
		segs = append(segs, "")
		block_closed = false
	}

	pat := regexp.MustCompile(`([*_])`)
	// pat1 := regexp.MustCompile(`\*\*(.*?)\*\*`)
	// pat2 := regexp.MustCompile(`__(.*?)__`)

	for i, seg := range segs {
		if (i % 2) == 0 { // not in code block
			ss := strings.Split(seg, "`") // WARN: does not handle escaped backticks
			if (len(ss) % 2) == 1 {       // backticks balanced
				for j, s := range ss {
					if (j % 2) == 0 { // not in inline code
						// // replace **...** with <strong>...</strong>
						// s = pat1.ReplaceAllString(s, `<strong>$1</strong>`)
						// // replace __...__ with <em>...</em>
						// s = pat2.ReplaceAllString(s, `<em>$1</em>`)
						// replace * with \* and _ with \_
						ss[j] = pat.ReplaceAllString(s, `\$1`)
					}
				}
			}
			segs[i] = strings.Join(ss, "`")
		}
	}

	text = strings.Join(segs, "```")
	return text, block_closed
}
