// Package correlation implements bounded, explicit variable expansion and JSON
// Pointer lookup. It never evaluates expressions or recursively expands values.
package correlation

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

const MaxValueBytes = 1 << 20

type part struct{ literal, name, filter string }
type Template struct{ parts []part }

func NameValid(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for i, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func Compile(s string, known map[string]bool) (Template, error) {
	t := Template{}
	for s != "" {
		at := strings.Index(s, "{{")
		if at < 0 {
			if strings.Contains(s, "}}") {
				return t, fmt.Errorf("unmatched template delimiter")
			}
			t.parts = append(t.parts, part{literal: s})
			break
		}
		if strings.Contains(s[:at], "}}") {
			return t, fmt.Errorf("unmatched template delimiter")
		}
		if at > 0 {
			t.parts = append(t.parts, part{literal: s[:at]})
		}
		s = s[at+2:]
		end := strings.Index(s, "}}")
		if end < 0 {
			return t, fmt.Errorf("unclosed variable placeholder")
		}
		bits := strings.Split(strings.TrimSpace(s[:end]), "|")
		name := strings.TrimSpace(bits[0])
		if len(bits) > 2 || !NameValid(name) {
			return t, fmt.Errorf("invalid variable placeholder")
		}
		filter := ""
		if len(bits) == 2 {
			filter = strings.TrimSpace(bits[1])
			if filter != "url" && filter != "path" && filter != "json" {
				return t, fmt.Errorf("unsupported variable filter")
			}
		}
		if !known[name] {
			return t, fmt.Errorf("variable %q is not defined before this step", name)
		}
		t.parts = append(t.parts, part{name: name, filter: filter})
		s = s[end+2:]
	}
	return t, nil
}

func (t Template) Expand(values map[string]string) (string, error) {
	var out strings.Builder
	for _, p := range t.parts {
		value := p.literal
		if p.name != "" {
			var ok bool
			value, ok = values[p.name]
			if !ok {
				return "", fmt.Errorf("missing variable %q", p.name)
			}
			if len(value) > MaxValueBytes {
				return "", fmt.Errorf("variable exceeds 1 MiB")
			}
			switch p.filter {
			case "url":
				value = url.QueryEscape(value)
			case "path":
				value = url.PathEscape(value)
			case "json":
				b, _ := json.Marshal(value)
				value = string(b)
			}
		}
		if out.Len()+len(value) > MaxValueBytes {
			return "", fmt.Errorf("expanded field exceeds 1 MiB")
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

type Pointer struct{ tokens []string }

func ParsePointer(s string) (Pointer, error) {
	p := Pointer{}
	if s == "" {
		return p, nil
	}
	if !strings.HasPrefix(s, "/") {
		return p, fmt.Errorf("JSON pointer must be empty or begin with /")
	}
	for _, token := range strings.Split(s[1:], "/") {
		var decoded strings.Builder
		for i := 0; i < len(token); i++ {
			if token[i] != '~' {
				decoded.WriteByte(token[i])
				continue
			}
			i++
			if i >= len(token) || (token[i] != '0' && token[i] != '1') {
				return p, fmt.Errorf("JSON pointer has an invalid ~ escape")
			}
			if token[i] == '0' {
				decoded.WriteByte('~')
			} else {
				decoded.WriteByte('/')
			}
		}
		p.tokens = append(p.tokens, decoded.String())
	}
	return p, nil
}

func DecodeJSON(body []byte) (any, error) {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("response is not valid JSON")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("response contains trailing JSON data")
	}
	return value, nil
}

// Scalar preserves numeric precision and rejects missing, null, or structured
// values rather than silently turning them into unusable authentication data.
func (p Pointer) Scalar(value any) (string, error) {
	for _, key := range p.tokens {
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[key]
			if !ok {
				return "", fmt.Errorf("JSON pointer does not exist")
			}
		case []any:
			if key == "" || (len(key) > 1 && key[0] == '0') {
				return "", fmt.Errorf("invalid JSON array index")
			}
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(v) || strconv.Itoa(i) != key {
				return "", fmt.Errorf("invalid JSON array index")
			}
			value = v[i]
		default:
			return "", fmt.Errorf("JSON pointer cannot traverse scalar value")
		}
	}
	switch v := value.(type) {
	case string:
		return v, nil
	case json.Number:
		return string(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", fmt.Errorf("JSON pointer must select a non-null scalar")
	}
}
