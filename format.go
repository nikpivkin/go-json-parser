package main

import (
	"fmt"
	"strings"
)

func astToString(el *jsonElement) string {
	var (
		indent int
		sb     strings.Builder
		walk   func(e *jsonElement)
	)

	write := func(s string) {
		sb.WriteString(strings.Repeat(" ", indent))
		sb.WriteString(s)
	}

	walk = func(e *jsonElement) {
		write(e.kind.String())
		sb.WriteString(":")
		if e.kind == objectKind || e.kind == arrayKind {
			indent++
			sb.WriteString("\n")
		}

		switch v := e.value.(type) {
		case *jsonElement:
			walk(v)
		case []*jsonElement:
			for _, el := range v {
				walk(el)
			}
		case []*pair:
			for _, p := range v {
				write("key:")
				sb.WriteString(string(p.key))
				sb.WriteString("\n")
				write("value: ")
				sb.WriteString("\n")
				indent++
				walk(p.value)
				indent--
			}
		case bool:
			sb.WriteString(fmt.Sprintf("%v", e.value))
		default:
			sb.WriteString(fmt.Sprintf("%s", e.value))
		}

		sb.WriteString("\n")
		if e.kind == objectKind {
			indent--
		}
	}

	walk(el)

	return sb.String()
}

func minify(e *jsonElement) string {
	var (
		sb   strings.Builder
		walk func(el *jsonElement)
	)
	walk = func(e *jsonElement) {
		if e.kind == objectKind {
			sb.WriteRune('{')
		} else if e.kind == arrayKind {
			sb.WriteRune('[')
		}

		switch e.kind {
		case arrayKind:
			val := e.value.([]*jsonElement)
			for i, el := range val {
				walk(el)
				if i != len(val)-1 {
					sb.WriteRune(',')
				}
			}
		case objectKind:
			val := e.value.([]*pair)
			for i, p := range val {
				sb.WriteRune('"')
				sb.WriteString(string(p.key))
				sb.WriteRune('"')
				sb.WriteRune(':')
				walk(p.value)
				if i != len(val)-1 {
					sb.WriteRune(',')
				}
			}
		case stringKind:
			sb.WriteRune('"')
			sb.WriteString(string(e.value.([]byte)))
			sb.WriteRune('"')
		case numberKind:
			sb.WriteString(fmt.Sprintf("%s", e.value))
		case booleanKind:
			sb.WriteString(fmt.Sprintf("%v", e.value))
		case nullKind:
			sb.WriteString("null")
		default:
			panic("unreachable")
		}

		if e.kind == objectKind {
			sb.WriteRune('}')
		} else if e.kind == arrayKind {
			sb.WriteRune(']')
		}
	}

	walk(e)

	return sb.String()
}

func pretty(e *jsonElement, indent int) string {
	var (
		sb        strings.Builder
		walk      func(el *jsonElement)
		lvl       int
		ignoreLvl bool
	)

	write := func(s string) {
		if !ignoreLvl {
			sb.WriteString(strings.Repeat(" ", lvl*indent))
		} else {
			sb.WriteRune(' ')
		}
		sb.WriteString(s)
		ignoreLvl = false
	}

	walk = func(e *jsonElement) {
		switch e.kind {
		case arrayKind:
			write("[")
			val := e.value.([]*jsonElement)

			if len(val) == 0 {
				sb.WriteRune(']')
				return
			}

			sb.WriteRune('\n')
			lvl++

			for i, el := range val {
				walk(el)
				if i != len(val)-1 {
					sb.WriteRune(',')
				}
				sb.WriteRune('\n')
			}
			lvl--
			write("]")
		case objectKind:
			write("{")
			val := e.value.([]*pair)
			if len(val) == 0 {
				sb.WriteRune('}')
				return
			}

			sb.WriteRune('\n')
			lvl++
			for i, p := range val {
				write(`"`)
				sb.WriteString(string(p.key))
				sb.WriteRune('"')
				sb.WriteRune(':')
				ignoreLvl = true
				walk(p.value)
				if i != len(val)-1 {
					sb.WriteRune(',')
				}
				sb.WriteRune('\n')
			}
			lvl--
			write("}")

		case stringKind:
			write(`"`)
			sb.WriteString(string(e.value.([]byte)))
			sb.WriteRune('"')
		case numberKind:
			write(fmt.Sprintf("%s", e.value))
		case booleanKind:
			write(fmt.Sprintf("%v", e.value))
		case nullKind:
			write("null")
		default:
			panic("unreachable")
		}
	}

	walk(e)

	return sb.String()
}
