// Package jmx analyzes JMeter plans without executing scripts, opening referenced
// files, resolving external entities, or sending network requests.
package jmx

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const MaxSourceBytes = 1 << 20
const maxXMLNodes = 20000
const maxComponents = 500

type element struct {
	kind       string
	attrs      map[string]string
	text       string
	textBuffer []byte
	children   []*element
}

func (e *element) prop(name string) *element {
	for _, c := range e.children {
		if c.attrs["name"] == name {
			return c
		}
	}
	return nil
}
func (e *element) value(name string) string {
	if p := e.prop(name); p != nil {
		return p.text
	}
	return ""
}
func (e *element) descendants(kind string) []*element {
	var result []*element
	for _, c := range e.children {
		if c.kind == kind {
			result = append(result, c)
		}
		result = append(result, c.descendants(kind)...)
	}
	return result
}

func parse(r io.Reader) (*element, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSourceBytes {
		return nil, fmt.Errorf("JMX file exceeds 1 MiB")
	}
	d := xml.NewDecoder(strings.NewReader(string(data)))
	var root *element
	var stack []*element
	nodes := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid JMX XML at line %d", line(d))
		}
		switch t := token.(type) {
		case xml.StartElement:
			nodes++
			if nodes > maxXMLNodes || len(stack) >= 64 {
				return nil, fmt.Errorf("JMX structure exceeds the node or nesting limit")
			}
			if t.Name.Space != "" {
				return nil, fmt.Errorf("XML namespaces are not supported in JMX")
			}
			e := &element{kind: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				if a.Name.Space != "" {
					return nil, fmt.Errorf("XML namespaces are not supported in JMX")
				}
				if _, exists := e.attrs[a.Name.Local]; exists {
					return nil, fmt.Errorf("duplicate XML attribute")
				}
				e.attrs[a.Name.Local] = a.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("JMX must contain exactly one document")
				}
				root = e
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, e)
			}
			stack = append(stack, e)
		case xml.EndElement:
			e := stack[len(stack)-1]
			e.text = string(e.textBuffer)
			e.textBuffer = nil
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				e := stack[len(stack)-1]
				e.textBuffer = append(e.textBuffer, t...)
			} else if strings.TrimSpace(string(t)) != "" {
				return nil, fmt.Errorf("unexpected text outside JMX document")
			}
		case xml.Directive:
			return nil, fmt.Errorf("DTD and XML directives are not allowed")
		case xml.ProcInst:
			if t.Target != "xml" || root != nil {
				return nil, fmt.Errorf("XML processing instructions are not allowed")
			}
		}
	}
	if root == nil || root.kind != "jmeterTestPlan" || root.attrs["version"] != "1.2" {
		return nil, fmt.Errorf("expected a JMeter jmeterTestPlan with XML format version 1.2")
	}
	if len(root.children) != 1 || root.children[0].kind != "hashTree" {
		return nil, fmt.Errorf("JMX root must contain one hashTree")
	}
	return root, nil
}

func line(d *xml.Decoder) int { l, _ := d.InputPos(); return l }

type tree struct {
	e        *element
	children []*tree
	id       string
	path     string
	index    int
}

func buildTree(hash *element, parent string, n *int) ([]*tree, error) {
	if strings.TrimSpace(hash.text) != "" || len(hash.children)%2 != 0 {
		return nil, fmt.Errorf("each JMX component must be followed by a hashTree")
	}
	var result []*tree
	for i := 0; i < len(hash.children); i += 2 {
		e, children := hash.children[i], hash.children[i+1]
		if e.kind == "hashTree" || children.kind != "hashTree" {
			return nil, fmt.Errorf("each JMX component must be followed by a hashTree")
		}
		*n++
		if *n > maxComponents {
			return nil, fmt.Errorf("JMX exceeds 500 components")
		}
		name := e.attrs["testname"]
		if name == "" {
			name = e.kind
		}
		if len(name) > 256 {
			return nil, fmt.Errorf("JMX component names must not exceed 256 bytes")
		}
		path := name
		if parent != "" {
			path = parent + " / " + name
		}
		t := &tree{e: e, id: fmt.Sprintf("component-%d", *n), path: path, index: -1}
		var err error
		t.children, err = buildTree(children, path, n)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, nil
}
