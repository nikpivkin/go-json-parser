package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode/utf8"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err.Error())
	}
}

func run() error {
	mode := flag.String("mode", "ast", "one of ast|pretty|minify")
	flag.Parse()

	if len(flag.Args()) < 1 {
		return errors.New("path to JSON is required")
	}
	b, err := os.ReadFile(flag.Args()[0])
	if err != nil {
		return err
	}
	p := newParser(b)
	json, err := p.parse()
	if err != nil {
		return err
	}

	switch *mode {
	case "ast":
		fmt.Println(astToString(json))
	case "pretty":
		fmt.Println(pretty(json, 2))
	case "minify":
		fmt.Println(minify(json))
	default:
		panic(fmt.Sprintf("unsupported mode: %q", *mode))
	}
	return nil
}

type elementKind uint8

func (k elementKind) String() string {
	switch k {
	case objectKind:
		return "object"
	case arrayKind:
		return "array"
	case stringKind:
		return "string"
	case numberKind:
		return "number"
	case booleanKind:
		return "boolean"
	case nullKind:
		return "null"
	}
	panic("unreachable")
}

const (
	objectKind elementKind = iota + 1
	arrayKind
	stringKind
	numberKind
	booleanKind
	nullKind
)

type pair struct {
	key   []byte
	value *jsonElement
}

type jsonElement struct {
	kind  elementKind
	value any
}

type reader struct {
	s      []byte
	line   int
	col    int
	offset int
}

func (r *reader) isEOF() bool {
	return r.offset >= len(r.s)
}

func (r *reader) peek() (v rune, size int) {
	if r.isEOF() {
		return v, 0
	}
	v, size = rune(r.s[r.offset]), 1
	// check if rune is base ASCII character
	if v >= 128 {
		v, size = utf8.DecodeRune(r.s[r.offset:])
		if v == utf8.RuneError && size == 1 {
			v = rune(r.s[r.offset]) // illegal encoding
		}
	}
	return v, size
}

func (r *reader) read() (v rune) {
	v, s := r.peek()
	if r.isEOF() {
		return v
	}
	if v == '\n' {
		r.col = 0
		r.line++
	} else {
		r.col++
	}
	r.offset += s
	return v
}

type parser struct {
	r reader
}

func newParser(s []byte) *parser {
	return &parser{
		r: reader{s: s, line: 1, col: 0},
	}
}

func (p *parser) parse() (*jsonElement, error) {
	return p.parseRoot()
}

func (p *parser) parseRoot() (*jsonElement, error) {
	p.eatWhitespace()
	root, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	p.eatWhitespace()
	if !p.r.isEOF() {
		return nil, p.expectedError("eof", p.r.read())
	}

	return root, nil
}

func (p *parser) parseValue() (el *jsonElement, err error) {
	r := p.r.read()
	switch r {
	case '{':
		el, err = p.parseObject()
	case '[':
		el, err = p.parseArray()
	case '"':
		el, err = p.parseString()
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		el, err = p.parseNumber(r)
	case 't', 'f':
		el, err = p.parseBool(r)
	case 'n':
		el, err = p.parseNull()
	default:
		return el, p.syntaxError(
			fmt.Errorf("unexpected token: %q", r),
		)
	}
	return
}

func (p *parser) parseObject() (*jsonElement, error) {
	p.eatWhitespace()

	var members []*pair

	for !p.r.isEOF() {
		if r, _ := p.r.peek(); r == '}' {
			break
		}

		if len(members) != 0 {
			r := p.r.read()
			if r != ',' {
				return nil, p.expectedError(",", r)
			}
			p.eatWhitespace()
		}

		member, err := p.parseMember()
		if err != nil {
			return nil, err
		}

		if member == nil && len(members) != 0 {
			return nil, p.syntaxError(fmt.Errorf("expected object member"))
		} else if member == nil {
			break
		}

		members = append(members, member)
	}

	if r := p.r.read(); r != '}' {
		return nil, p.expectedError("}", r)
	}

	return &jsonElement{
		kind:  objectKind,
		value: members,
	}, nil
}

func (p *parser) parseMember() (*pair, error) {
	r, _ := p.r.peek()

	if r != '"' {
		return nil, nil
	}

	p.r.read()

	key, err := p.parseRawString()
	if err != nil {
		return nil, err
	}
	p.eatWhitespace()

	if r = p.r.read(); r != ':' {
		return nil, p.expectedError(":", r)
	}

	p.eatWhitespace()

	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	p.eatWhitespace()

	return &pair{
		key:   key,
		value: val,
	}, nil
}

func (p *parser) parseArray() (*jsonElement, error) {
	p.eatWhitespace()

	var elements []*jsonElement

	for !p.r.isEOF() {
		if r, _ := p.r.peek(); r == ']' {
			break
		}

		if len(elements) != 0 {
			r := p.r.read()
			if r != ',' {
				return nil, p.expectedError(",", r)
			}
			p.eatWhitespace()
		}

		el, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		elements = append(elements, el)

		p.eatWhitespace()
	}

	p.eatWhitespace()

	if r := p.r.read(); r != ']' {
		return nil, p.expectedError("]", r)
	}

	return &jsonElement{
		kind:  arrayKind,
		value: elements,
	}, nil
}

func (p *parser) parseString() (*jsonElement, error) {
	raw, err := p.parseRawString()
	if err != nil {
		return nil, err
	}
	return &jsonElement{
		kind:  stringKind,
		value: raw,
	}, nil
}

func (p *parser) parseRawString() ([]byte, error) {
	if r, _ := p.r.peek(); r == '"' {
		p.r.read()
		return []byte{}, nil
	}

	start := p.r.offset
	var escape bool
	for !p.r.isEOF() {
		r := p.r.read()
		if !escape && r == '"' {
			break
		}

		if !escape && isSpecialCharacter(r) {
			return nil, p.syntaxError(fmt.Errorf("unescaped special caharacter %q", r))
		}

		if escape {
			switch r {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			case 'u':
				for range 4 {
					if !isHex(p.r.read()) {
						return nil, p.expectedError("hexadecimal digit", r)
					}
				}
			default:
				return nil, p.syntaxError(fmt.Errorf("invalid escape character %q", r))
			}
		}

		if !escape && r == '\\' {
			escape = true
		} else {
			escape = false
		}

	}

	if start == p.r.offset {
		return nil, p.syntaxError(fmt.Errorf("expected: \", but 'eof'"))
	}
	return p.r.s[start : p.r.offset-1], nil
}

func isSpecialCharacter(r rune) bool {
	return r >= 0 && r <= 31
}

func isHex(r rune) bool {
	return isDigit(r) || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
}

func (p *parser) parseNumber(start rune) (*jsonElement, error) {
	var sb strings.Builder
	sb.WriteRune(start)

	if err := p.parseInteger(start, &sb); err != nil {
		return nil, err
	}

	if err := p.parseFraction(&sb); err != nil {
		return nil, err
	}

	if err := p.parseExponent(&sb); err != nil {
		return nil, err
	}

	return &jsonElement{
		kind:  numberKind,
		value: sb.String(),
	}, nil
}

func (p *parser) parseInteger(start rune, sb *strings.Builder) error {
	if start == '-' {
		r := p.r.read()
		if r == '0' {
			sb.WriteRune(r)
			return nil
		}
		if !isNaturalDigit(r) {
			return p.expectedError("digit '1-9'", r)
		}
		sb.WriteRune(r)
	}

	if start != '0' {
		for !p.r.isEOF() {
			r, _ := p.r.peek()
			if !isDigit(r) {
				break
			}
			sb.WriteRune(p.r.read())
		}
	}

	return nil
}

func (p *parser) parseFraction(sb *strings.Builder) error {
	r, _ := p.r.peek()
	if r == '.' {
		sb.WriteRune('.')
		p.r.read()
		var hasDigit bool
		for !p.r.isEOF() {
			r, _ := p.r.peek()
			if !isDigit(r) {
				break
			}
			hasDigit = true
			sb.WriteRune(p.r.read())
		}

		if !hasDigit {
			return p.syntaxError(fmt.Errorf("expected: digit after fraction '.'"))
		}
	}

	return nil
}

func (p *parser) parseExponent(sb *strings.Builder) error {
	r, _ := p.r.peek()
	if r == 'e' || r == 'E' {
		sb.WriteRune(r)
		p.r.read()
		r, _ = p.r.peek()
		if r == '+' || r == '-' {
			p.r.read()
			sb.WriteRune(r)
		}

		var hasDigit bool

		for !p.r.isEOF() {
			r, _ := p.r.peek()
			if !isDigit(r) {
				break
			}
			hasDigit = true
			sb.WriteRune(p.r.read())
		}

		if !hasDigit {
			return p.syntaxError(fmt.Errorf("expected: digit after exponent"))
		}
	}

	return nil
}

func isNaturalDigit(r rune) bool {
	return r >= '1' && r <= '9'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func (p *parser) parseBool(start rune) (*jsonElement, error) {
	switch start {
	case 't':
		ok, expected, got := p.match("rue")
		if ok {
			return &jsonElement{
				kind:  booleanKind,
				value: true,
			}, nil
		}

		return nil, p.expectedError(string(expected), got)
	case 'f':
		ok, expected, got := p.match("alse")
		if ok {
			return &jsonElement{
				kind:  booleanKind,
				value: false,
			}, nil
		}

		return nil, p.expectedError(string(expected), got)
	default:
		panic("unreachable")
	}
}

func (p *parser) parseNull() (*jsonElement, error) {
	ok, expected, got := p.match("ull")
	if ok {
		return &jsonElement{
			kind: nullKind,
		}, nil
	}

	return nil, p.expectedError(string(expected), got)
}

func (p *parser) eatWhitespace() {
	for !p.r.isEOF() {
		r, _ := p.r.peek()

		if !isWhitespace(r) {
			break
		}

		p.r.read()
	}
}

func isWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func (p *parser) match(s string) (bool, rune, rune) {
	for _, ss := range s {
		r := p.r.read()
		if r != ss {
			return false, ss, r
		}
	}

	return true, 0, 0
}

func (p *parser) expectedError(expected string, got rune) error {
	return p.syntaxError(
		fmt.Errorf(
			"expected: %q, but got: %q",
			expected, string(got)),
	)
}

func (p *parser) syntaxError(err error) error {
	return fmt.Errorf(
		"syntax error in JSON at line %d, column %d: %w", p.r.line, p.r.col, err,
	)
}
