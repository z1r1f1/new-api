package openai

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/MoeForge/moego/ojson"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// normalizeResponsesToolSchemas translates legacy nested function definitions
// to the flat Responses format and closes object schemas as required by the
// upstream Responses API. Batch raw edits preserve schema property order,
// precise numbers and opaque provider-specific tool members. Unchanged input
// is returned without copying.
func normalizeResponsesToolSchemas(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !ojson.ValidBytes(raw) {
		return raw
	}
	tools := ojson.GetBytesNoCopy(raw, "@this")
	if !tools.IsArray() {
		return raw
	}
	doc := ojson.NewDoc(raw)
	changed := false
	for index, tool := range tools.All() {
		if !tool.IsObject() {
			if tool.Type == ojson.Null {
				continue
			}
			return raw
		}
		if tool.Get("type").Str != "function" {
			continue
		}
		updated, err := normalizeResponsesFunctionTool(doc, strconv.Itoa(int(index.Int())), tool)
		if err != nil {
			return raw
		}
		changed = changed || updated
	}
	if !changed {
		return raw
	}
	return doc.Bytes()
}

func normalizeResponsesFunctionTool(doc *ojson.Doc, path string, tool ojson.Result) (bool, error) {
	changed := false
	name := tool.Get("name").Str
	parameters := tool.Get("parameters")
	if function := tool.Get("function"); function.IsObject() {
		if strings.TrimSpace(name) == "" {
			if nestedName := strings.TrimSpace(function.Get("name").Str); nestedName != "" {
				if err := relaycommon.SetJSONMember(doc, path, "name", nestedName); err != nil {
					return false, err
				}
				name = nestedName
			}
		}
		if strings.TrimSpace(tool.Get("description").Str) == "" {
			if description := function.Get("description"); strings.TrimSpace(description.Str) != "" {
				if err := doc.SetRaw(path+".description", description.Raw); err != nil {
					return false, err
				}
			}
		}
		if !parameters.Exists() {
			if nestedParameters := function.Get("parameters"); nestedParameters.Exists() {
				if err := doc.SetRaw(path+".parameters", nestedParameters.Raw); err != nil {
					return false, err
				}
				parameters = nestedParameters
			}
		}
		if err := relaycommon.DeleteJSONMember(doc, path+".function"); err != nil {
			return false, err
		}
		changed = true
	}
	updated, err := closeResponsesObjectSchema(doc, path+".parameters", parameters)
	if err != nil {
		return false, err
	}
	changed = changed || updated
	if strings.EqualFold(strings.TrimSpace(name), "Read") {
		updated, err = removeReadPagesParameter(doc, path+".parameters")
		if err != nil {
			return false, err
		}
		changed = changed || updated
	}
	return changed, nil
}

func closeResponsesObjectSchema(doc *ojson.Doc, path string, parameters ojson.Result) (bool, error) {
	if !parameters.IsObject() || strings.TrimSpace(parameters.Raw[1:len(parameters.Raw)-1]) == "" {
		return true, doc.SetRaw(path, `{"type":"object","properties":{},"additionalProperties":false}`)
	}
	return closeResponsesSchemaValue(doc, path, parameters)
}

func closeResponsesSchemaValue(doc *ojson.Doc, path string, value ojson.Result) (bool, error) {
	changed := false
	if value.IsArray() {
		for index, item := range value.All() {
			updated, err := closeResponsesSchemaValue(doc, path+"."+strconv.Itoa(int(index.Int())), item)
			if err != nil {
				return false, err
			}
			changed = changed || updated
		}
		return changed, nil
	}
	if !value.IsObject() {
		return false, nil
	}
	for key, child := range value.All() {
		childPath := path + "." + ojson.Escape(key.Str)
		if child.Type == ojson.Null || key.Str == "required" && !child.IsArray() {
			if err := relaycommon.DeleteJSONMember(doc, childPath); err != nil {
				return false, err
			}
			changed = true
			continue
		}
		switch key.Str {
		case "properties":
			if child.IsObject() {
				for name, property := range child.All() {
					updated, err := closeResponsesSchemaValue(doc, childPath+"."+ojson.Escape(name.Str), property)
					if err != nil {
						return false, err
					}
					changed = changed || updated
				}
			}
		case "items", "anyOf", "oneOf", "allOf":
			if key.Str == "items" || child.IsArray() {
				updated, err := closeResponsesSchemaValue(doc, childPath, child)
				if err != nil {
					return false, err
				}
				changed = changed || updated
			}
		}
	}
	if properties := value.Get("properties"); properties.Exists() && properties.Type != ojson.Null {
		if value.Get("additionalProperties").Type == ojson.Null {
			if err := relaycommon.SetJSONMember(doc, path, "additionalProperties", false); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

func removeReadPagesParameter(doc *ojson.Doc, path string) (bool, error) {
	changed := false
	if properties := doc.Get(path + ".properties"); properties.IsObject() && properties.Get("pages").Exists() {
		if err := relaycommon.DeleteJSONMember(doc, path+".properties.pages"); err != nil {
			return false, err
		}
		changed = true
	}
	if required := doc.Get(path + ".required"); required.IsArray() {
		removed, remaining := 0, 0
		for index, item := range required.All() {
			if item.Type != ojson.String || item.Str != "pages" {
				remaining++
				continue
			}
			if err := doc.Delete(path + ".required." + strconv.Itoa(int(index.Int())-removed)); err != nil {
				return false, err
			}
			removed++
			changed = true
		}
		if remaining == 0 {
			if err := relaycommon.DeleteJSONMember(doc, path+".required"); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}
