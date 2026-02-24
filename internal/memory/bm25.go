package memory

import (
	"math"
	"strings"
	"unicode"
)

// BM25 parameters
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25Index is an in-process BM25 search index.
type BM25Index struct {
	docs     []indexedDoc
	avgDL    float64 // average document length
	df       map[string]int // document frequency per term
	docCount int
}

type indexedDoc struct {
	id     string
	terms  map[string]int // term frequencies
	length int            // number of terms
}

// NewBM25Index creates an empty BM25 index.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		df: make(map[string]int),
	}
}

// Add indexes a document with the given ID and text content.
func (idx *BM25Index) Add(id, text string) {
	tokens := tokenize(text)
	tf := make(map[string]int)
	for _, tok := range tokens {
		tf[tok]++
	}

	// Track unique terms for document frequency
	for term := range tf {
		idx.df[term]++
	}

	idx.docs = append(idx.docs, indexedDoc{
		id:     id,
		terms:  tf,
		length: len(tokens),
	})
	idx.docCount++

	// Recompute average document length
	totalLen := 0
	for _, d := range idx.docs {
		totalLen += d.length
	}
	idx.avgDL = float64(totalLen) / float64(idx.docCount)
}

// Search returns document IDs ranked by BM25 relevance to the query.
func (idx *BM25Index) Search(query string, maxResults int) []SearchResult {
	if idx.docCount == 0 {
		return nil
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	type scoredDoc struct {
		id    string
		score float64
	}

	var results []scoredDoc

	for _, doc := range idx.docs {
		score := 0.0
		for _, term := range queryTerms {
			tf := float64(doc.terms[term])
			if tf == 0 {
				continue
			}

			df := float64(idx.df[term])
			// IDF with smoothing
			idf := math.Log(1 + (float64(idx.docCount)-df+0.5)/(df+0.5))

			// BM25 TF normalization
			tfNorm := (tf * (bm25K1 + 1)) / (tf + bm25K1*(1-bm25B+bm25B*float64(doc.length)/idx.avgDL))

			score += idf * tfNorm
		}

		if score > 0 {
			results = append(results, scoredDoc{id: doc.id, score: score})
		}
	}

	// Sort by score descending
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = SearchResult{ID: r.id, Score: r.score}
	}
	return out
}

// SearchResult holds a search result with its relevance score.
type SearchResult struct {
	ID    string
	Score float64
}

// tokenize splits text into lowercase word tokens, removing punctuation.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tok := current.String()
				if len(tok) >= 2 { // skip single-char tokens
					tokens = append(tokens, tok)
				}
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tok := current.String()
		if len(tok) >= 2 {
			tokens = append(tokens, tok)
		}
	}

	return tokens
}
