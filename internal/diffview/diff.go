package diffview

import "strings"

type Segment struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func LineDiff(before, after string) []Segment {
	a := splitLines(before)
	b := splitLines(after)

	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}

	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	segments := make([]Segment, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			segments = appendSegment(segments, "equal", a[i])
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			segments = appendSegment(segments, "delete", a[i])
			i++
		default:
			segments = appendSegment(segments, "insert", b[j])
			j++
		}
	}

	for ; i < len(a); i++ {
		segments = appendSegment(segments, "delete", a[i])
	}
	for ; j < len(b); j++ {
		segments = appendSegment(segments, "insert", b[j])
	}
	return segments
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func appendSegment(items []Segment, kind, text string) []Segment {
	if len(items) > 0 && items[len(items)-1].Type == kind {
		items[len(items)-1].Text += "\n" + text
		return items
	}
	return append(items, Segment{Type: kind, Text: text})
}
