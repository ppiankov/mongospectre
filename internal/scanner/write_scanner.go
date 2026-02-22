package scanner

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var writeOperationRe = regexp.MustCompile(`(?i)\.(insertone|insertmany|insert_one|insert_many|replaceone|replace_one|findoneandreplace|find_one_and_replace|updateone|updatemany|update_one|update_many|findoneandupdate|find_one_and_update|bulkwrite|bulk_write)\(`)

var numberLiteralRe = regexp.MustCompile(`^-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?$`)

var updateWriteOperators = map[string]bool{
	"$set":         true,
	"$setoninsert": true,
	"$inc":         true,
	"$mul":         true,
	"$min":         true,
	"$max":         true,
	"$unset":       true,
	"$push":        true,
	"$addtoset":    true,
	"$rename":      true,
}

// writeFieldMatch records a written field and inferred value type from one line.
type writeFieldMatch struct {
	Field     string
	ValueType string
}

type kvPair struct {
	key   string
	value string
}

// IsWriteOperation reports whether a line contains a MongoDB write operation.
func IsWriteOperation(line string) bool {
	return writeOperationRe.MatchString(line)
}

// ScanLineWriteFields extracts written fields and inferred types from one line.
func ScanLineWriteFields(line string) []writeFieldMatch {
	bounds := writeOperationRe.FindStringSubmatchIndex(line)
	if len(bounds) < 4 {
		return nil
	}

	op := strings.ToLower(line[bounds[2]:bounds[3]])
	openParen := bounds[1] - 1
	if openParen < 0 || openParen >= len(line) || line[openParen] != '(' {
		return nil
	}

	args := splitCallArgs(line, openParen)
	scopes := writeScopesForOperation(op, args)
	if len(scopes) == 0 {
		return nil
	}

	seen := make(map[string]string)
	for _, scope := range scopes {
		for _, pair := range parseAssignments(scope) {
			field := strings.TrimSpace(pair.key)
			if !isValidFieldName(field) || strings.HasPrefix(field, "$") {
				continue
			}
			valueType := inferWriteValueType(pair.value)
			if prev, ok := seen[field]; ok {
				if prev == ValueTypeUnknown && valueType != ValueTypeUnknown {
					seen[field] = valueType
				}
				continue
			}
			seen[field] = valueType
		}
	}

	out := make([]writeFieldMatch, 0, len(seen))
	for field, valueType := range seen {
		out = append(out, writeFieldMatch{Field: field, ValueType: valueType})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Field < out[j].Field
	})
	return out
}

func writeScopesForOperation(op string, args []string) []string {
	switch op {
	case "insertone", "insert_one", "insertmany", "insert_many":
		var scopes []string
		for _, arg := range args {
			if !looksLikeDocumentExpr(arg) {
				continue
			}
			scopes = append(scopes, arg)
			if op != "insertmany" && op != "insert_many" {
				break
			}
		}
		return scopes
	case "replaceone", "replace_one", "findoneandreplace", "find_one_and_replace":
		if len(args) < 2 {
			return nil
		}
		if !looksLikeDocumentExpr(args[1]) {
			return nil
		}
		return []string{args[1]}
	case "updateone", "updatemany", "update_one", "update_many", "findoneandupdate", "find_one_and_update":
		if len(args) < 2 {
			return nil
		}
		return updateScopes(args[1])
	default:
		return nil
	}
}

func updateScopes(expr string) []string {
	pairs := parseAssignments(expr)
	if len(pairs) == 0 {
		if looksLikeDocumentExpr(expr) {
			return []string{expr}
		}
		return nil
	}

	hasUpdateOperator := false
	var scopes []string
	for _, pair := range pairs {
		key := strings.ToLower(strings.TrimSpace(pair.key))
		if !strings.HasPrefix(key, "$") {
			continue
		}
		hasUpdateOperator = true
		if updateWriteOperators[key] && looksLikeDocumentExpr(pair.value) {
			scopes = append(scopes, pair.value)
		}
	}

	if hasUpdateOperator {
		return scopes
	}
	if looksLikeDocumentExpr(expr) {
		return []string{expr}
	}
	return nil
}

func splitCallArgs(line string, openParen int) []string {
	if openParen < 0 || openParen >= len(line) || line[openParen] != '(' {
		return nil
	}

	var (
		args     []string
		inStr    byte
		escaped  bool
		paren    = 1
		braces   = 0
		brackets = 0
		argStart = openParen + 1
	)

	for i := openParen + 1; i < len(line); i++ {
		c := line[i]
		if inStr != 0 {
			if inStr != '`' && escaped {
				escaped = false
				continue
			}
			if inStr != '`' && c == '\\' {
				escaped = true
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}

		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(':
			paren++
		case ')':
			paren--
			if paren == 0 {
				if arg := strings.TrimSpace(line[argStart:i]); arg != "" {
					args = append(args, arg)
				}
				return args
			}
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case ',':
			if paren == 1 && braces == 0 && brackets == 0 {
				if arg := strings.TrimSpace(line[argStart:i]); arg != "" {
					args = append(args, arg)
				}
				argStart = i + 1
			}
		}
	}
	return args
}

func looksLikeDocumentExpr(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	lower := strings.ToLower(expr)
	if strings.HasPrefix(expr, "{") || strings.HasPrefix(expr, "[") {
		return true
	}
	if strings.HasPrefix(lower, "bson.m{") || strings.HasPrefix(lower, "bson.d{") || strings.HasPrefix(lower, "bson.a{") {
		return true
	}
	if strings.HasPrefix(lower, "map[") {
		return true
	}
	return strings.Contains(expr, "{") && strings.Contains(expr, ":")
}

func parseAssignments(expr string) []kvPair {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}

	if strings.HasPrefix(expr, "[") {
		body, ok := enclosedBody(expr, '[', ']')
		if !ok {
			return nil
		}
		var pairs []kvPair
		for _, elem := range splitTopLevel(body) {
			pairs = append(pairs, parseObjectAssignments(elem)...)
		}
		return pairs
	}
	return parseObjectAssignments(expr)
}

func parseObjectAssignments(expr string) []kvPair {
	body, ok := enclosedBody(expr, '{', '}')
	if !ok {
		return nil
	}
	return parsePairsBody(body)
}

func enclosedBody(expr string, open, close byte) (string, bool) {
	start := strings.IndexByte(expr, open)
	if start == -1 {
		return "", false
	}
	end := findMatching(expr, start, open, close)
	if end == -1 || end <= start {
		return "", false
	}
	return expr[start+1 : end], true
}

func findMatching(s string, start int, open, close byte) int {
	depth := 0
	inStr := byte(0)
	escaped := false

	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if inStr != '`' && escaped {
				escaped = false
				continue
			}
			if inStr != '`' && c == '\\' {
				escaped = true
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}

		switch c {
		case '"', '\'', '`':
			inStr = c
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevel(body string) []string {
	var (
		parts   []string
		inStr   byte
		escaped bool
		braces  int
		bracks  int
		parens  int
		start   int
	)

	for i := 0; i < len(body); i++ {
		c := body[i]
		if inStr != 0 {
			if inStr != '`' && escaped {
				escaped = false
				continue
			}
			if inStr != '`' && c == '\\' {
				escaped = true
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}

		switch c {
		case '"', '\'', '`':
			inStr = c
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		case '[':
			bracks++
		case ']':
			if bracks > 0 {
				bracks--
			}
		case '(':
			parens++
		case ')':
			if parens > 0 {
				parens--
			}
		case ',':
			if braces == 0 && bracks == 0 && parens == 0 {
				if part := strings.TrimSpace(body[start:i]); part != "" {
					parts = append(parts, part)
				}
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(body[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func parsePairsBody(body string) []kvPair {
	var pairs []kvPair
	for _, part := range splitTopLevel(body) {
		key, value, ok := splitPair(part)
		if !ok {
			continue
		}
		pairs = append(pairs, kvPair{key: key, value: value})
	}
	return pairs
}

func splitPair(part string) (string, string, bool) {
	inStr := byte(0)
	escaped := false
	braces := 0
	bracks := 0
	parens := 0

	for i := 0; i < len(part); i++ {
		c := part[i]
		if inStr != 0 {
			if inStr != '`' && escaped {
				escaped = false
				continue
			}
			if inStr != '`' && c == '\\' {
				escaped = true
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		case '[':
			bracks++
		case ']':
			if bracks > 0 {
				bracks--
			}
		case '(':
			parens++
		case ')':
			if parens > 0 {
				parens--
			}
		case ':':
			if braces == 0 && bracks == 0 && parens == 0 {
				key := strings.TrimSpace(part[:i])
				value := strings.TrimSpace(part[i+1:])
				if key == "" || value == "" {
					return "", "", false
				}
				key = trimQuotes(key)
				if key == "" || !isIdentifierToken(key) {
					return "", "", false
				}
				return key, value, true
			}
		}
	}
	return "", "", false
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') ||
			(s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func isIdentifierToken(s string) bool {
	for i, r := range s {
		if i == 0 {
			if unicode.IsLetter(r) || r == '_' || r == '$' {
				continue
			}
			return false
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '$' {
			continue
		}
		return false
	}
	return len(s) > 0
}

func inferWriteValueType(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ValueTypeUnknown
	}
	lower := strings.ToLower(v)

	switch {
	case strings.HasPrefix(v, "{") || strings.HasPrefix(lower, "bson.m{") || strings.HasPrefix(lower, "bson.d{") || strings.HasPrefix(lower, "map["):
		return ValueTypeObject
	case strings.HasPrefix(v, "[") || strings.HasPrefix(lower, "bson.a{"):
		return ValueTypeArray
	case strings.HasPrefix(v, "\"") || strings.HasPrefix(v, "'") || strings.HasPrefix(v, "`"):
		return ValueTypeString
	case startsWithToken(lower, "true") || startsWithToken(lower, "false"):
		return ValueTypeBool
	case startsWithToken(lower, "null") || startsWithToken(lower, "nil"):
		return ValueTypeNull
	case strings.HasPrefix(lower, "primitive.newobjectid") || strings.HasPrefix(lower, "objectid("):
		return ValueTypeObjectID
	case strings.HasPrefix(lower, "time.") || strings.HasPrefix(lower, "new date(") || strings.HasPrefix(lower, "isodate("):
		return ValueTypeDate
	case numberLiteralRe.MatchString(v):
		return ValueTypeNumber
	default:
		return ValueTypeUnknown
	}
}

func startsWithToken(s, token string) bool {
	if !strings.HasPrefix(s, token) {
		return false
	}
	if len(s) == len(token) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[len(token):])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
}
