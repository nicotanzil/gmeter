package telemetry

import (
	"bufio"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Value struct {
	Value *float64 `json:"value,omitempty"`
	Rate  *float64 `json:"rate,omitempty"`
	Issue string   `json:"issue,omitempty"`
	Reset bool     `json:"reset,omitempty"`
}

func number(v float64) *float64 { return &v }
func finite(v float64) bool     { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Only legacy ASCII metric/label names and scalar text samples are supported.
// Unselected families are skipped; selected malformed/duplicate series fail
// visibly. Explicit exporter timestamps cannot establish current observations.
func parse(data []byte, metrics []Metric) ([]Value, error) {
	if !utf8.Valid(data) || (len(data) > 0 && data[len(data)-1] != '\n') {
		return nil, fmt.Errorf("telemetry text must be UTF-8 with a final newline")
	}
	values := make([]Value, len(metrics))
	matched := make([]int, len(metrics))
	types := map[string]string{}
	for i := range values {
		values[i].Issue = "selected series missing"
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line[0] == '#' {
			parts := strings.Fields(strings.TrimSpace(line[1:]))
			if len(parts) > 0 && parts[0] == "TYPE" {
				if len(parts) != 3 {
					return nil, fmt.Errorf("invalid telemetry TYPE metadata")
				}
				for _, m := range metrics {
					if m.Name == parts[1] {
						if types[m.Name] != "" {
							return nil, fmt.Errorf("duplicate selected TYPE metadata")
						}
						types[m.Name] = parts[2]
						break
					}
				}
			}
			continue
		}
		end := strings.IndexAny(line, "{ \t")
		if end < 0 {
			end = len(line)
		}
		name := line[:end]
		selected := false
		for _, m := range metrics {
			selected = selected || m.Name == name
		}
		if !selected {
			continue
		}
		labels, rest, err := sampleLabels(line[end:])
		if err != nil {
			return nil, fmt.Errorf("invalid selected telemetry labels")
		}
		parts := strings.Fields(rest)
		if len(parts) < 1 || len(parts) > 2 {
			return nil, fmt.Errorf("invalid selected telemetry sample")
		}
		v, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid selected telemetry value")
		}
		for i, m := range metrics {
			if m.Name != name || !maps.Equal(m.Labels, labels) {
				continue
			}
			matched[i]++
			if matched[i] > 1 {
				values[i] = Value{Issue: "duplicate selected series"}
				continue
			}
			if len(parts) == 2 {
				values[i] = Value{Issue: "exporter timestamps are unsupported"}
				continue
			}
			if !finite(v) || math.Abs(v) > 1e100 || (m.Kind == "counter" && v < 0) {
				values[i] = Value{Issue: "non-finite or invalid selected value"}
				continue
			}
			values[i] = Value{Value: number(v)}
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("telemetry line exceeds 64 KiB")
	}
	for i, m := range metrics {
		if kind := types[m.Name]; kind != "" && kind != "untyped" && kind != m.Kind {
			values[i] = Value{Issue: "exporter TYPE differs from configured kind"}
		}
	}
	return values, nil
}

func sampleLabels(rest string) (map[string]string, string, error) {
	labels := map[string]string{}
	if trimmed := strings.TrimLeft(rest, " \t"); strings.HasPrefix(trimmed, "{") {
		rest = trimmed
	}
	if !strings.HasPrefix(rest, "{") {
		if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			return nil, "", fmt.Errorf("missing sample separator")
		}
		return labels, rest, nil
	}
	rest = rest[1:]
	for {
		rest = strings.TrimLeft(rest, " \t")
		if strings.HasPrefix(rest, "}") {
			rest = rest[1:]
			break
		}
		i := strings.IndexAny(rest, "= \t")
		if i < 1 {
			return nil, "", fmt.Errorf("invalid label name")
		}
		key := rest[:i]
		if !labelName.MatchString(key) || len(labels) >= 64 {
			return nil, "", fmt.Errorf("invalid labels")
		}
		if _, ok := labels[key]; ok {
			return nil, "", fmt.Errorf("duplicate label")
		}
		rest = strings.TrimLeft(rest[i:], " \t")
		if !strings.HasPrefix(rest, "=") {
			return nil, "", fmt.Errorf("missing equals")
		}
		rest = strings.TrimLeft(rest[1:], " \t")
		if !strings.HasPrefix(rest, `"`) {
			return nil, "", fmt.Errorf("missing quote")
		}
		rest = rest[1:]
		var value strings.Builder
		closed := false
		for len(rest) > 0 {
			ch := rest[0]
			rest = rest[1:]
			if ch == '"' {
				closed = true
				break
			}
			if ch == '\\' {
				if len(rest) == 0 {
					return nil, "", fmt.Errorf("unfinished escape")
				}
				ch = rest[0]
				rest = rest[1:]
				switch ch {
				case 'n':
					ch = '\n'
				case '\\', '"':
				default:
					return nil, "", fmt.Errorf("unsupported escape")
				}
			}
			value.WriteByte(ch)
		}
		if !closed {
			return nil, "", fmt.Errorf("unfinished label")
		}
		labels[key] = value.String()
		rest = strings.TrimLeft(rest, " \t")
		if strings.HasPrefix(rest, ",") {
			rest = rest[1:]
			continue
		}
		if !strings.HasPrefix(rest, "}") {
			return nil, "", fmt.Errorf("missing label separator")
		}
	}
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return nil, "", fmt.Errorf("missing value separator")
	}
	return labels, rest, nil
}
