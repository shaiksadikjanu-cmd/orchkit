package nodes

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"orchkit"
)

// XML parses an XML string into a map structure, or converts a map to XML.
//
// Example parse:
//
//	nodes.NewXML("parse")
//
// Example build:
//
//	nodes.NewXML("build")
type XML struct {
	Mode string // "parse" or "build"
}

func NewXML(mode string) *XML {
	if mode == "" {
		mode = "parse"
	}
	return &XML{Mode: mode}
}

func (x *XML) Name() string { return "xml" }

func (x *XML) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Parses XML to a map (mode=parse) or converts a JSON object to XML (mode=build).",
		Params: map[string]any{
			"text": map[string]any{"type": "string", "desc": "XML string to parse (mode=parse), or JSON to convert (mode=build)."},
			"mode": map[string]any{"type": "string", "desc": "parse or build. Defaults to parse."},
			"root": map[string]any{"type": "string", "desc": "Root element name for build mode. Defaults to 'root'."},
		},
	}
}

func (x *XML) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	mode := x.Mode
	if v, ok := in["mode"].(string); ok && v != "" {
		mode = v
	}

	text, _ := in["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("xml: 'text' is required")
	}

	switch mode {
	case "parse":
		result, err := parseXML([]byte(text))
		if err != nil {
			return nil, fmt.Errorf("xml: parse: %w", err)
		}
		return orchkit.Output{"result": result}, nil

	case "build":
		root := "root"
		if v, ok := in["root"].(string); ok && v != "" {
			root = v
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(text), &data); err != nil {
			return nil, fmt.Errorf("xml: build input must be valid JSON: %w", err)
		}
		xmlStr := buildXML(root, data)
		return orchkit.Output{"xml": xmlStr}, nil

	default:
		return nil, fmt.Errorf("xml: unknown mode %q (use parse or build)", mode)
	}
}

func parseXML(data []byte) (map[string]any, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var stack []map[string]any
	var root map[string]any

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			node := map[string]any{"_tag": t.Name.Local}
			for _, attr := range t.Attr {
				node["@"+attr.Name.Local] = attr.Value
			}
			stack = append(stack, node)
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if len(stack) > 0 && text != "" {
				stack[len(stack)-1]["_text"] = text
			}
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				root = node
			} else {
				parent := stack[len(stack)-1]
				tag := node["_tag"].(string)
				if existing, ok := parent[tag]; ok {
					switch v := existing.(type) {
					case []any:
						parent[tag] = append(v, node)
					default:
						parent[tag] = []any{v, node}
					}
				} else {
					parent[tag] = node
				}
			}
		}
	}
	return root, nil
}

func buildXML(tag string, data map[string]any) string {
	var sb strings.Builder
	sb.WriteString("<" + tag + ">")
	for k, v := range data {
		switch val := v.(type) {
		case map[string]any:
			sb.WriteString(buildXML(k, val))
		case string:
			sb.WriteString("<" + k + ">" + val + "</" + k + ">")
		default:
			sb.WriteString(fmt.Sprintf("<%s>%v</%s>", k, val, k))
		}
	}
	sb.WriteString("</" + tag + ">")
	return sb.String()
}
