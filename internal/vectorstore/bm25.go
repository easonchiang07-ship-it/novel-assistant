package vectorstore

import (
	"math"
	"strings"
	"unicode"
)

const (
	bm25k1 = 1.5
	bm25b  = 0.75
)

// tokenize splits text into lowercase tokens.
// CJK characters are treated as individual tokens; other letters/digits
// are grouped into words. Punctuation and whitespace act as delimiters.
func tokenize(text string) []string {
	var tokens []string
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			tokens = append(tokens, string(buf))
			buf = buf[:0]
		}
	}
	for _, r := range strings.ToLower(text) {
		if isCJK(r) {
			flush()
			tokens = append(tokens, string(r))
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf = append(buf, r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

type bm25Index struct {
	tf     map[string]map[string]int // docID -> term -> count
	df     map[string]int            // term -> document frequency
	docLen map[string]int            // docID -> token count
	avgdl  float64
	n      int
}

func buildBM25Index(docs []Document) *bm25Index {
	idx := &bm25Index{
		tf:     make(map[string]map[string]int),
		df:     make(map[string]int),
		docLen: make(map[string]int),
	}
	for _, d := range docs {
		terms := tokenize(d.Content)
		tf := make(map[string]int, len(terms))
		for _, t := range terms {
			tf[t]++
		}
		idx.tf[d.ID] = tf
		idx.docLen[d.ID] = len(terms)
		for t := range tf {
			idx.df[t]++
		}
	}
	idx.n = len(docs)
	if idx.n > 0 {
		total := 0
		for _, l := range idx.docLen {
			total += l
		}
		idx.avgdl = float64(total) / float64(idx.n)
	}
	return idx
}

// score computes BM25 score for a document given pre-tokenized query terms.
func (idx *bm25Index) score(queryTerms []string, docID string) float64 {
	if idx.n == 0 || idx.avgdl == 0 {
		return 0
	}
	docTerms := idx.tf[docID]
	docLen := idx.docLen[docID]
	var s float64
	for _, term := range queryTerms {
		df := float64(idx.df[term])
		if df == 0 {
			continue
		}
		idf := math.Log((float64(idx.n)-df+0.5)/(df+0.5) + 1)
		tf := float64(docTerms[term])
		norm := tf + bm25k1*(1-bm25b+bm25b*float64(docLen)/idx.avgdl)
		s += idf * (tf * (bm25k1 + 1)) / norm
	}
	return s
}
