package main

import (
	"go/scanner"
	"go/token"
	"html/template"
	"strings"
)

// Token classes used in the rendered HTML. Kept to one letter because every
// token in the report carries one, and none of them collide with the row
// classes (n, g, t, cov, uncov, add, off, gap).
const (
	classKeyword    = "k"
	classString     = "s"
	classNumber     = "m"
	classComment    = "c"
	classPredeclare = "b"
	classFuncName   = "f"
)

// predeclared are the identifiers in Go's universe block. They are not
// keywords, so the scanner reports them as plain identifiers.
var predeclared = map[string]bool{
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true, "float32": true,
	"float64": true, "int": true, "int8": true, "int16": true, "int32": true,
	"int64": true, "rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true,

	"false": true, "iota": true, "nil": true, "true": true,

	"append": true, "cap": true, "clear": true, "close": true,
	"complex": true, "copy": true, "delete": true, "imag": true, "len": true,
	"make": true, "max": true, "min": true, "new": true, "panic": true,
	"print": true, "println": true, "real": true, "recover": true,
}

// tokenSpan is a half-open byte range of src that should be wrapped in class.
type tokenSpan struct {
	start, end int
	class      string
}

// highlightLines splits src into one HTML fragment per line, matching
// strings.Split(src, "\n") element for element. Go sources get their tokens
// wrapped in class spans; anything else — and any source the scanner chokes
// on — falls back to plain escaped text.
func highlightLines(relPath, sourceText string) []template.HTML {
	if !strings.HasSuffix(relPath, ".go") {
		return plainLines(sourceText)
	}
	spans, ok := goTokenSpans(sourceText)
	if !ok {
		return plainLines(sourceText)
	}
	return assembleLines(sourceText, spans)
}

func plainLines(sourceText string) []template.HTML {
	sourceLines := strings.Split(sourceText, "\n")
	escaped := make([]template.HTML, len(sourceLines))
	for index, line := range sourceLines {
		escaped[index] = template.HTML(template.HTMLEscapeString(line))
	}
	return escaped
}

// scanned is one token, with the source range it actually occupies.
type scanned struct {
	tok        token.Token
	literal    string
	start, end int
}

// goTokenSpans tokenizes src and returns the ranges worth colouring, in
// source order and never overlapping. It reports false when the scanner
// finds a syntax error, so a source that cannot be tokenized cleanly renders
// as plain text instead of being mis-coloured.
func goTokenSpans(sourceText string) ([]tokenSpan, bool) {
	tokens, ok := scanTokens(sourceText)
	if !ok {
		return nil, false
	}
	isFuncName := funcNameIndexes(tokens)

	spans := make([]tokenSpan, 0, len(tokens))
	for index, scannedToken := range tokens {
		class := ""
		switch {
		case scannedToken.tok == token.COMMENT:
			class = classComment
		case scannedToken.tok == token.STRING, scannedToken.tok == token.CHAR:
			class = classString
		case scannedToken.tok == token.INT, scannedToken.tok == token.FLOAT,
			scannedToken.tok == token.IMAG:
			class = classNumber
		case scannedToken.tok.IsKeyword():
			class = classKeyword
		case isFuncName[index]:
			class = classFuncName
		case scannedToken.tok == token.IDENT && predeclared[scannedToken.literal]:
			class = classPredeclare
		}
		if class == "" {
			continue
		}
		spans = append(spans, tokenSpan{scannedToken.start, scannedToken.end, class})
	}
	return spans, true
}

func scanTokens(sourceText string) ([]scanned, bool) {
	fileSet := token.NewFileSet()
	file := fileSet.AddFile("", fileSet.Base(), len(sourceText))

	sawError := false
	var goScanner scanner.Scanner
	goScanner.Init(file, []byte(sourceText),
		func(token.Position, string) { sawError = true }, scanner.ScanComments)

	var tokens []scanned
	for {
		position, tok, literal := goScanner.Scan()
		if tok == token.EOF || sawError {
			break
		}
		// Semicolons the scanner inserts at end of line are not in the source.
		if tok == token.SEMICOLON && literal == "\n" {
			continue
		}

		text := literal
		if text == "" {
			text = tok.String()
		}
		start := file.Offset(position)
		end := start + len(text)
		if tok == token.COMMENT {
			// Comment literals have carriage returns stripped, so their length
			// can disagree with the source; measure the delimiters instead.
			end = commentEnd(sourceText, start)
		}
		if start < 0 || start >= len(sourceText) || end <= start {
			continue
		}
		if end > len(sourceText) {
			end = len(sourceText)
		}
		// A literal whose reported length undershot the source would otherwise
		// leave this token's range running into the previous one.
		if count := len(tokens); count > 0 && tokens[count-1].end > start {
			tokens[count-1].end = start
		}
		tokens = append(tokens, scanned{tok, literal, start, end})
	}
	if sawError {
		return nil, false
	}
	return tokens, true
}

// funcNameIndexes marks the identifiers that name a declared function or
// method. A declaration's name is the identifier after `func` — or after a
// parenthesized receiver — and is always followed by the parameter list, or
// by a type parameter list. That last check is what keeps the return type of
// a func *type* (`var f func(int) int`) from being mistaken for a name.
func funcNameIndexes(tokens []scanned) map[int]bool {
	isFuncName := map[int]bool{}
	for index, scannedToken := range tokens {
		if scannedToken.tok != token.FUNC {
			continue
		}
		nameIndex := index + 1
		if nameIndex < len(tokens) && tokens[nameIndex].tok == token.LPAREN {
			nameIndex = skipParens(tokens, nameIndex)
		}
		if nameIndex+1 >= len(tokens) || tokens[nameIndex].tok != token.IDENT {
			continue
		}
		next := tokens[nameIndex+1].tok
		if next == token.LPAREN || next == token.LBRACK {
			isFuncName[nameIndex] = true
		}
	}
	return isFuncName
}

// skipParens returns the index just past the parenthesis group opening at
// openIndex.
func skipParens(tokens []scanned, openIndex int) int {
	depth := 0
	for ; openIndex < len(tokens); openIndex++ {
		switch tokens[openIndex].tok {
		case token.LPAREN:
			depth++
		case token.RPAREN:
			if depth--; depth == 0 {
				return openIndex + 1
			}
		}
	}
	return openIndex
}

// commentEnd returns the offset just past the comment starting at start.
func commentEnd(sourceText string, start int) int {
	if strings.HasPrefix(sourceText[start:], "//") {
		if newlineIndex := strings.IndexByte(sourceText[start:], '\n'); newlineIndex >= 0 {
			return start + newlineIndex
		}
		return len(sourceText)
	}
	if closeIndex := strings.Index(sourceText[start+2:], "*/"); closeIndex >= 0 {
		return start + 2 + closeIndex + 2
	}
	return len(sourceText)
}

// assembleLines walks src once, emitting escaped text and wrapping the spans,
// and cuts a new line at every newline. Tokens that span lines — raw strings
// and block comments — are wrapped separately on each line they cover, since
// every line is its own row in the report.
func assembleLines(sourceText string, spans []tokenSpan) []template.HTML {
	lines := make([]template.HTML, 0, strings.Count(sourceText, "\n")+1)
	var currentLine strings.Builder

	emit := func(text, class string) {
		for {
			chunk, rest := text, ""
			endsLine := false
			if newlineIndex := strings.IndexByte(text, '\n'); newlineIndex >= 0 {
				chunk, rest, endsLine = text[:newlineIndex], text[newlineIndex+1:], true
			}
			if chunk != "" {
				if class != "" {
					currentLine.WriteString(`<span class="` + class + `">`)
				}
				currentLine.WriteString(template.HTMLEscapeString(chunk))
				if class != "" {
					currentLine.WriteString(`</span>`)
				}
			}
			if !endsLine {
				return
			}
			lines = append(lines, template.HTML(currentLine.String()))
			currentLine.Reset()
			text = rest
		}
	}

	offset := 0
	for _, span := range spans {
		if span.start < offset {
			continue
		}
		emit(sourceText[offset:span.start], "")
		emit(sourceText[span.start:span.end], span.class)
		offset = span.end
	}
	emit(sourceText[offset:], "")
	return append(lines, template.HTML(currentLine.String()))
}
