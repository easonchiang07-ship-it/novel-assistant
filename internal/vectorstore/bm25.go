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
	tf       map[string]map[string]int // docID -> term -> count
	df       map[string]int            // term -> document frequency
	docLen   map[string]int            // docID -> token count
	totalLen int                       // sum of all doc lengths (for avgdl)
	n        int
}

func newBM25Index() *bm25Index {
	return &bm25Index{
		tf:     make(map[string]map[string]int),
		df:     make(map[string]int),
		docLen: make(map[string]int),
	}
}

func buildBM25Index(docs []Document) *bm25Index {
	idx := newBM25Index()
	for _, d := range docs {
		idx.addDoc(d)
	}
	return idx
}

func (idx *bm25Index) avgdl() float64 {
	if idx.n == 0 {
		return 0
	}
	return float64(idx.totalLen) / float64(idx.n)
}

// addDoc indexes a document. The caller must ensure the ID is not already present.
func (idx *bm25Index) addDoc(doc Document) {
	terms := tokenize(doc.Content)
	tf := make(map[string]int, len(terms))
	for _, t := range terms {
		tf[t]++
	}
	idx.tf[doc.ID] = tf
	idx.docLen[doc.ID] = len(terms)
	idx.totalLen += len(terms)
	idx.n++
	for t := range tf {
		idx.df[t]++
	}
}

// removeDoc removes a document from the index. No-op if the ID is not present.
func (idx *bm25Index) removeDoc(id string) {
	tf, ok := idx.tf[id]
	if !ok {
		return
	}
	for term := range tf {
		idx.df[term]--
		if idx.df[term] == 0 {
			delete(idx.df, term)
		}
	}
	idx.totalLen -= idx.docLen[id]
	idx.n--
	delete(idx.tf, id)
	delete(idx.docLen, id)
}

// upsert adds or replaces a document incrementally (O(unique terms in doc)).
func (idx *bm25Index) upsert(doc Document) {
	idx.removeDoc(doc.ID)
	idx.addDoc(doc)
}

// score computes BM25 score for a document given pre-tokenized query terms.
func (idx *bm25Index) score(queryTerms []string, docID string) float64 {
	avgdl := idx.avgdl()
	if idx.n == 0 || avgdl == 0 {
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
		norm := tf + bm25k1*(1-bm25b+bm25b*float64(docLen)/avgdl)
		s += idf * (tf * (bm25k1 + 1)) / norm
	}
	return s
}
