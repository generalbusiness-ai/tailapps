package profile

import (
	"errors"
	"strings"
	"unicode"
)

func splitStatements(source string) ([]string, error) {
	var statements []string
	var current strings.Builder
	quote := rune(0)
	lineComment, blockComment := false, false
	runes := []rune(source)
	for index := 0; index < len(runes); index++ {
		char := runes[index]
		next := rune(0)
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		switch {
		case lineComment:
			if char == '\n' {
				lineComment = false
				current.WriteRune(char)
			}
		case blockComment:
			if char == '*' && next == '/' {
				blockComment = false
				index++
				current.WriteByte(' ')
			}
		case quote != 0:
			current.WriteRune(char)
			if char == quote {
				if next == quote {
					current.WriteRune(next)
					index++
				} else {
					quote = 0
				}
			}
		case char == '-' && next == '-':
			lineComment = true
			index++
		case char == '/' && next == '*':
			blockComment = true
			index++
		case char == '\'' || char == '"':
			quote = char
			current.WriteRune(char)
		case char == ';':
			if statement := strings.TrimSpace(current.String()); statement != "" {
				statements = append(statements, statement)
			}
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}
	if quote != 0 || blockComment {
		return nil, errors.New("application SQL has an unterminated quote or comment")
	}
	if statement := strings.TrimSpace(current.String()); statement != "" {
		return nil, errors.New("application SQL statement is missing its semicolon")
	}
	return statements, nil
}

func splitComma(source string) ([]string, error) {
	var parts []string
	var current strings.Builder
	depth := 0
	quote := rune(0)
	for _, char := range source {
		switch {
		case quote != 0:
			current.WriteRune(char)
			if char == quote {
				quote = 0
			}
		case char == '\'' || char == '"':
			quote = char
			current.WriteRune(char)
		case char == '(':
			depth++
			current.WriteRune(char)
		case char == ')':
			depth--
			if depth < 0 {
				return nil, errors.New("unbalanced parentheses")
			}
			current.WriteRune(char)
		case char == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}
	if quote != 0 || depth != 0 {
		return nil, errors.New("unbalanced declaration")
	}
	parts = append(parts, strings.TrimSpace(current.String()))
	for _, part := range parts {
		if part == "" || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return nil, errors.New("empty or invalid declaration item")
		}
	}
	return parts, nil
}

// normalizeSQLTokens gives storage compatibility a formatting-independent
// representation while retaining every identifier, literal, operator and
// constraint token. The admitted DDL does not permit quoted identifiers, but
// quoted CHECK literals are preserved exactly.
func normalizeSQLTokens(source string) string {
	runes := []rune(source)
	var tokens []string
	for index := 0; index < len(runes); {
		char := runes[index]
		if unicode.IsSpace(char) {
			index++
			continue
		}
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' {
			start := index
			for index < len(runes) && (unicode.IsLetter(runes[index]) || unicode.IsDigit(runes[index]) || runes[index] == '_') {
				index++
			}
			tokens = append(tokens, strings.ToLower(string(runes[start:index])))
			continue
		}
		if char == '\'' || char == '"' {
			quote := char
			start := index
			index++
			for index < len(runes) {
				if runes[index] == quote {
					index++
					if index < len(runes) && runes[index] == quote {
						index++
						continue
					}
					break
				}
				index++
			}
			tokens = append(tokens, string(runes[start:index]))
			continue
		}
		if index+1 < len(runes) {
			pair := string(runes[index : index+2])
			if pair == ">=" || pair == "<=" || pair == "<>" || pair == "!=" || pair == "||" {
				tokens = append(tokens, pair)
				index += 2
				continue
			}
		}
		tokens = append(tokens, string(char))
		index++
	}
	return strings.Join(tokens, "\x1f")
}
