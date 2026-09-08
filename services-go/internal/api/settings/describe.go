package settings

import "encoding/json"

// Field is one setting as the admin page sees it: what it is, and what it is
// currently set to.
type Field struct {
	Definition
	// Value is what is stored, or the default when nothing is. Absent for a
	// secret.
	Value any `json:"value,omitempty"`
	// IsSet says whether a secret has a value, without saying what it is.
	IsSet *bool `json:"isSet,omitempty"`
}

// Description is everything the admin page needs to draw itself.
type Description struct {
	Sections []Section `json:"sections"`
	Fields   []Field   `json:"fields"`
}

// Describe answers what the admin page needs.
//
// A secret is reported as set or not set rather than by value: an
// administrator who can open the page is not necessarily somebody who should
// be handed the SMTP password back.
func (s *Store) Describe() Description {
	stored := s.current.Load().raw
	fields := make([]Field, 0, len(Settings))

	for _, definition := range Settings {
		field := Field{Definition: definition}
		raw, present := stored[definition.Key]

		if definition.Secret {
			isSet := present && len(raw) > 0 && string(raw) != `""` && string(raw) != "null"
			field.IsSet = &isSet
			fields = append(fields, field)
			continue
		}

		if present {
			var value any
			if err := json.Unmarshal(raw, &value); err == nil {
				field.Value = value
			}
		}
		if field.Value == nil {
			field.Value = definition.Default
		}
		fields = append(fields, field)
	}

	return Description{Sections: Sections, Fields: fields}
}

// Apply stores a set of changes, refusing anything the catalogue does not
// name.
//
// An empty secret means "leave it alone", because that is what the page shows
// for one that is already set; clearing one is done by sending "-", which is
// not a password anybody meant to use.
func (s *Store) Apply(changes map[string]any) map[string]any {
	applied := make(map[string]any, len(changes))
	for key, value := range changes {
		definition, known := DefinitionFor(key)
		if !known {
			continue
		}
		if definition.Secret {
			text, _ := value.(string)
			if text == "" {
				continue
			}
			if text == "-" {
				applied[key] = ""
				continue
			}
		}
		applied[key] = value
	}
	return applied
}
