package input

import (
	"fmt"
	"strconv"
	"strings"
)

// Tokenize splits a -keys string into tokens.
//
// Tokens are separated by unquoted whitespace. Double or single quotes group
// text containing spaces and can appear inside a token (type:"hello world");
// the quotes themselves are removed. Backslash escapes are processed inside
// and outside quotes: \t \n \r \e (ESC) \s (space) \\ \" \' and \xHH. An
// unknown escape is kept as-is, backslash included.
func Tokenize(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inToken := false
	var quote rune

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\':
			decoded, consumed, err := decodeEscape(runes[i:])
			if err != nil {
				return nil, err
			}
			cur.WriteString(decoded)
			i += consumed - 1
			inToken = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inToken = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote in %q", quote, s)
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

// Unescape processes backslash escapes (as in Tokenize) in s without
// splitting or treating quotes specially.
func Unescape(s string) (string, error) {
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '\\' {
			b.WriteRune(runes[i])
			continue
		}
		decoded, consumed, err := decodeEscape(runes[i:])
		if err != nil {
			return "", err
		}
		b.WriteString(decoded)
		i += consumed - 1
	}
	return b.String(), nil
}

// decodeEscape decodes the escape at the start of r (r[0] == '\\') and
// returns the decoded text and the number of runes consumed.
func decodeEscape(r []rune) (string, int, error) {
	if len(r) < 2 {
		return "\\", 1, nil
	}
	switch r[1] {
	case 't':
		return "\t", 2, nil
	case 'n':
		return "\n", 2, nil
	case 'r':
		return "\r", 2, nil
	case 'e':
		return "\x1b", 2, nil
	case 's':
		return " ", 2, nil
	case '\\', '"', '\'':
		return string(r[1]), 2, nil
	case 'x':
		if len(r) < 4 {
			return "", 0, fmt.Errorf("\\x needs two hex digits")
		}
		n, err := strconv.ParseUint(string(r[2:4]), 16, 8)
		if err != nil {
			return "", 0, fmt.Errorf("invalid escape \\x%s", string(r[2:4]))
		}
		return string([]byte{byte(n)}), 4, nil
	}
	return "\\" + string(r[1]), 2, nil
}
