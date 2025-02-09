// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package caddyfile

import (
	"bufio"
	"bytes"
	"io"
	"unicode"
)

type (
	// lexer is a utility which can get values, token by
	// token, from a Reader. A token is a word, and tokens
	// are separated by whitespace. A word can be enclosed
	// in quotes if it contains whitespace.
	lexer struct {
		reader       *bufio.Reader
		token        Token
		line         int
		skippedLines int
	}

	// Token represents a single parsable unit.
	Token struct {
		File        string
		origFile    string
		Line        int
		Text        string
		wasQuoted   rune // enclosing quote character, if any
		snippetName string
	}
)

// originalFile gets original filename before import modification.
func (t Token) originalFile() string {
	if t.origFile != "" {
		return t.origFile
	}
	return t.File
}

// updateFile updates the token's source filename for error display
// and remembers the original filename. Used during "import" processing.
func (t *Token) updateFile(file string) {
	if t.origFile == "" {
		t.origFile = t.File
	}
	t.File = file
}

// load prepares the lexer to scan an input for tokens.
// It discards any leading byte order mark.
func (l *lexer) load(input io.Reader) error {
	l.reader = bufio.NewReader(input)
	l.line = 1

	// discard byte order mark, if present
	firstCh, _, err := l.reader.ReadRune()
	if err != nil {
		return err
	}
	if firstCh != 0xFEFF {
		err := l.reader.UnreadRune()
		if err != nil {
			return err
		}
	}

	return nil
}

// next loads the next token into the lexer.
// A token is delimited by whitespace, unless
// the token starts with a quotes character (")
// in which case the token goes until the closing
// quotes (the enclosing quotes are not included).
// Inside quoted strings, quotes may be escaped
// with a preceding \ character. No other chars
// may be escaped. The rest of the line is skipped
// if a "#" character is read in. Returns true if
// a token was loaded; false otherwise.
func (l *lexer) next() bool {
	var val []rune
	var comment, quoted, btQuoted, escaped bool

	makeToken := func(quoted rune) bool {
		l.token.Text = string(val)
		l.token.wasQuoted = quoted
		return true
	}

	for {
		ch, _, err := l.reader.ReadRune()
		if err != nil {
			if len(val) > 0 {
				return makeToken(0)
			}
			if err == io.EOF {
				return false
			}
			panic(err)
		}

		if !escaped && !btQuoted && ch == '\\' {
			escaped = true
			continue
		}

		if quoted || btQuoted {
			if quoted && escaped {
				// all is literal in quoted area,
				// so only escape quotes
				if ch != '"' {
					val = append(val, '\\')
				}
				escaped = false
			} else {
				if quoted && ch == '"' {
					return makeToken('"')
				}
				if btQuoted && ch == '`' {
					return makeToken('`')
				}
			}
			if ch == '\n' {
				l.line += 1 + l.skippedLines
				l.skippedLines = 0
			}
			val = append(val, ch)
			continue
		}

		if unicode.IsSpace(ch) {
			if ch == '\r' {
				continue
			}
			if ch == '\n' {
				if escaped {
					l.skippedLines++
					escaped = false
				} else {
					l.line += 1 + l.skippedLines
					l.skippedLines = 0
				}
				comment = false
			}
			if len(val) > 0 {
				return makeToken(0)
			}
			continue
		}

		if ch == '#' && len(val) == 0 {
			comment = true
		}
		if comment {
			continue
		}

		if len(val) == 0 {
			l.token = Token{Line: l.line}
			if ch == '"' {
				quoted = true
				continue
			}
			if ch == '`' {
				btQuoted = true
				continue
			}
		}

		if escaped {
			val = append(val, '\\')
			escaped = false
		}

		val = append(val, ch)
	}
}

// Tokenize takes bytes as input and lexes it into
// a list of tokens that can be parsed as a Caddyfile.
// Also takes a filename to fill the token's File as
// the source of the tokens, which is important to
// determine relative paths for `import` directives.
func Tokenize(input []byte, filename string) ([]Token, error) {
	l := lexer{}
	if err := l.load(bytes.NewReader(input)); err != nil {
		return nil, err
	}
	var tokens []Token
	for l.next() {
		l.token.File = filename
		tokens = append(tokens, l.token)
	}
	return tokens, nil
}

func (t Token) Quoted() bool {
	return t.wasQuoted > 0
}
