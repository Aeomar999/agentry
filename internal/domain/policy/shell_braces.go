package policy

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// expandBraces replaces each word of a call or loop list with the words brace
// expansion makes of it, before anything else reads them: go test ./{a,b}/ is
// go test ./a/ ./b/. It rewrites the parsed file in place. A word it can't
// expand, and any brace that would expand in a declaration or a redirect,
// leaves the call unknown: not ok (declare x={a,} sets x to "", while x={a,}
// sets {a,}).
func expandBraces(file *syntax.File) (ok bool) {
	ok = true
	expandAll := func(words []*syntax.Word) []*syntax.Word {
		var all []*syntax.Word
		for _, word := range words {
			expanded, known := braceWords(word)
			if !known {
				ok = false
				expanded = []*syntax.Word{word}
			}
			all = append(all, expanded...)
		}
		return all
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			n.Args = expandAll(n.Args)
		case *syntax.WordIter:
			n.Items = expandAll(n.Items)
		case *syntax.DeclClause:
			for _, assign := range n.Args {
				if assign.Value != nil && !unbraced(assign.Value) {
					ok = false
				}
			}
		case *syntax.Redirect:
			if n.Word != nil && n.Hdoc == nil && n.Op != syntax.WordHdoc && !unbraced(n.Word) {
				ok = false
			}
		}
		return true
	})
	return ok
}

// unbraced: brace expansion leaves the word as it is.
func unbraced(word *syntax.Word) bool {
	words, ok := braceWords(word)
	return ok && len(words) == 1 && words[0] == word
}

// braceAtom is one piece of a word for brace expansion: a part that isn't a
// literal, or one character of a literal's text. brace marks a { , or } the
// shell reads as brace syntax: not quoted and not after a backslash.
type braceAtom struct {
	part  syntax.WordPart
	text  string
	brace bool
}

// mvdan's expand.Braces ignores backslashes, skips words with quotes and has
// no limit, so braceWords expands as bash does.
//
// braceWords is the words brace expansion makes of word: {a,b}c is ac bc,
// x{a,} is just xa, x{1..3} is x1 x2 x3. Not ok for a sequence this can't
// read ({1..a}), for more than maxValues words, for a word too long or with
// too many braces to expand quickly, and next to a $name or a $ that joins
// the next line, which bash reads again after expanding, so that $x{y,} is
// $xy $x.
func braceWords(word *syntax.Word) ([]*syntax.Word, bool) {
	atoms := braceAtoms(word)
	opens := 0
	for _, a := range atoms {
		if a.brace && a.text == "{" {
			opens++
		}
	}
	if opens == 0 {
		return []*syntax.Word{word}, true
	}
	if opens > maxBraces || len(atoms) > maxBracedLength {
		return nil, false
	}
	ways, ok := expandAtoms(atoms)
	if !ok || len(ways) > maxValues {
		return nil, false
	}
	if len(ways) == 1 && len(ways[0]) == len(atoms) { // nothing expanded; {5..5} did
		return []*syntax.Word{word}, true
	}
	if joinsDollar(word.Parts) || slices.ContainsFunc(word.Parts, func(part syntax.WordPart) bool {
		p, isVariable := part.(*syntax.ParamExp)
		return isVariable && p.Short
	}) {
		return nil, false
	}
	var words []*syntax.Word
	for _, way := range ways {
		if len(way) > 0 { // an empty one is no word at all
			words = append(words, wordOf(way))
		}
	}
	return words, true
}

func braceAtoms(word *syntax.Word) []braceAtom {
	var atoms []braceAtom
	for _, part := range word.Parts {
		lit, isLit := part.(*syntax.Lit)
		if !isLit {
			atoms = append(atoms, braceAtom{part: part})
			continue
		}
		for i := 0; i < len(lit.Value); i++ {
			if lit.Value[i] == '\\' && i+1 < len(lit.Value) {
				atoms = append(atoms, braceAtom{text: lit.Value[i : i+2]}) // kept for values to unescape
				i++
				continue
			}
			char := lit.Value[i : i+1]
			atoms = append(atoms, braceAtom{text: char, brace: strings.Contains("{,}", char)})
		}
	}
	return atoms
}

// maxBraces and maxBracedLength bound a word brace expansion reads, whose
// time grows with their product. The length counts the word's pieces, not its
// characters: an escape, a $x or a whole "…" is one piece.
const (
	maxBraces       = 256
	maxBracedLength = 4096
)

// expandAtoms expands the first {…} holding a comma at its own level, or a
// sequence, and the rest after it, as bash does. A { that doesn't close stays
// text, but a brace after it may still expand ({x{a,b}} is {xa} {xb}).
func expandAtoms(atoms []braceAtom) ([][]braceAtom, bool) {
	for open, a := range atoms {
		// bash passes a {} that starts the word through as text, so find
		// -exec's {} survives; after other text the { opens a brace (x{},a}
		// is x} xa).
		if !a.brace || a.text != "{" || open == 0 && isBrace(atoms, 1, "}") {
			continue
		}
		end, commas := closingBrace(atoms, open)
		if end < 0 {
			continue
		}
		choices, ok := braceChoices(atoms, open, end, commas)
		if !ok {
			return nil, false
		}
		rest, ok := expandAtoms(atoms[end+1:])
		if !ok || len(choices)*len(rest) > maxValues {
			return nil, false
		}
		var ways [][]braceAtom
		for _, choice := range choices {
			for _, after := range rest {
				ways = append(ways, slices.Concat(atoms[:open], choice, after))
			}
		}
		return ways, true
	}
	return [][]braceAtom{atoms}, true
}

// braceChoices is what one {…} expands to: each of its comma-separated parts
// expanded, or a sequence's terms. Not ok for a sequence this can't read.
func braceChoices(atoms []braceAtom, open, end int, commas []int) ([][]braceAtom, bool) {
	if len(commas) == 0 {
		return sequence(atoms[open+1 : end])
	}
	var choices [][]braceAtom
	start := open
	for _, stop := range append(commas, end) {
		parts, ok := expandAtoms(atoms[start+1 : stop])
		if !ok || len(choices)+len(parts) > maxValues {
			return nil, false
		}
		choices = append(choices, parts...)
		start = stop
	}
	return choices, true
}

// sequence is the terms of a brace sequence as bash makes them: {1..3} is
// 1 2 3, {3..1} is 3 2 1, {1..10..3} is 1 4 7 10, {01..03} is 01 02 03, and
// {a..c} is a b c. Not ok for any other {x..y}: a sign, a negative number
// padded with zeros, letters of mixed case, or a step that isn't a number.
func sequence(inner []braceAtom) ([][]braceAtom, bool) {
	for _, a := range inner {
		if a.part != nil || a.brace || len(a.text) != 1 {
			return nil, false
		}
	}
	ends := strings.Split(atomsText(inner), "..")
	if len(ends) != 2 && len(ends) != 3 {
		return nil, false
	}
	step := 1
	if len(ends) == 3 {
		var ok bool
		if step, ok = sequenceNumber(ends[2]); !ok {
			return nil, false
		}
		step = max(step, -step, 1)
	}
	first, last, format, ok := sequenceEnds(ends[0], ends[1])
	if !ok || (max(first, last)-min(first, last))/step+1 > maxValues {
		return nil, false
	}
	if last < first {
		step = -step
	}
	var terms [][]braceAtom
	for n := first; step > 0 && n <= last || step < 0 && n >= last; n += step {
		term := format(n)
		atoms := make([]braceAtom, len(term))
		for i := range term {
			atoms[i] = braceAtom{text: term[i : i+1]}
		}
		terms = append(terms, atoms)
	}
	return terms, true
}

// sequenceEnds reads a sequence's two ends, both numbers or both letters of
// one case, and how each term is written.
func sequenceEnds(from, to string) (first, last int, format func(int) string, ok bool) {
	isLetter := func(s string) bool { return len(s) == 1 && ('a' <= s[0] && s[0] <= 'z' || 'A' <= s[0] && s[0] <= 'Z') }
	if isLetter(from) && isLetter(to) {
		sameCase := ('a' <= from[0]) == ('a' <= to[0])
		return int(from[0]), int(to[0]), func(n int) string { return string(rune(n)) }, sameCase
	}
	first, okFirst := sequenceNumber(from)
	last, okLast := sequenceNumber(to)
	if !okFirst || !okLast {
		return 0, 0, nil, false
	}
	padded := func(s string) bool {
		digits := strings.TrimPrefix(s, "-")
		return len(digits) > 1 && digits[0] == '0'
	}
	if !padded(from) && !padded(to) {
		return first, last, strconv.Itoa, true
	}
	if first < 0 || last < 0 {
		return 0, 0, nil, false
	}
	width := max(len(from), len(to))
	return first, last, func(n int) string { return fmt.Sprintf("%0*d", width, n) }, true
}

// sequenceNumber reads a whole number of up to nine digits, maybe after a -.
func sequenceNumber(s string) (int, bool) {
	digits := strings.TrimPrefix(s, "-")
	if digits == "" || len(digits) > 9 || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// closingBrace is where the { at open closes, -1 when it doesn't, and the
// commas at its own level. As in bash, a } before any comma, which isn't the
// end of something like a sequence, leaves the { open: {a}b,c} is a}b c.
func closingBrace(atoms []braceAtom, open int) (end int, commas []int) {
	depth, dots := 0, false
	for i := open; i < len(atoms); i++ {
		switch {
		case !atoms[i].brace:
			dots = dots || atoms[i].text == "." && atoms[i-1].text == "."
		case atoms[i].text == "{":
			depth++
		case atoms[i].text == "}":
			depth--
			if depth > 0 {
				continue
			}
			if len(commas) > 0 || dots {
				return i, commas
			}
			depth = 1
		case depth == 1:
			commas = append(commas, i)
		}
	}
	return -1, nil
}

func isBrace(atoms []braceAtom, i int, brace string) bool {
	return i < len(atoms) && atoms[i].brace && atoms[i].text == brace
}

func atomsText(atoms []braceAtom) string {
	var b strings.Builder
	for _, a := range atoms {
		b.WriteString(a.text)
	}
	return b.String()
}

// wordOf joins atoms back into a word, each run of text into one literal.
func wordOf(atoms []braceAtom) *syntax.Word {
	word := &syntax.Word{}
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			word.Parts = append(word.Parts, &syntax.Lit{Value: text.String()})
			text.Reset()
		}
	}
	for _, a := range atoms {
		if a.part != nil {
			flush()
			word.Parts = append(word.Parts, a.part)
			continue
		}
		text.WriteString(a.text)
	}
	flush()
	return word
}
