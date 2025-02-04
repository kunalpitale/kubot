package template

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kubeshop/botkube/pkg/api"
)

type (
	// Template represents a template for message parsing.
	Template struct {
		Type         string       `yaml:"type"`
		Trigger      Trigger      `yaml:"trigger"`
		ParseMessage ParseMessage `yaml:"-"`
		WrapMessage  WrapMessage  `yaml:"-"`
	}

	// Trigger represents the trigger configuration for a template.
	Trigger struct {
		Command string `yaml:"command"`
	}

	// ParseMessage holds template for message that will be parsed by defined parser.
	ParseMessage struct {
		Selects []Select          `yaml:"selects"`
		Actions map[string]string `yaml:"actions"`
		Preview string            `yaml:"preview"`
	}

	// WrapMessage holds template for wrapping command output with additional context.
	WrapMessage struct {
		Buttons api.Buttons `yaml:"buttons"`
	}

	// Select holds template select primitive definition.
	Select struct {
		Name   string `yaml:"name"`
		KeyTpl string `yaml:"keyTpl"`
	}
)

// UnmarshalYAML is a custom unmarshaler for Template allowing to unmarshal into a proper struct
// base on defined template type.
func (su *Template) UnmarshalYAML(node *yaml.Node) error {
	var data struct {
		Type    string  `yaml:"type"`
		Trigger Trigger `yaml:"trigger"`
	}
	err := node.Decode(&data)
	if err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(data.Type, "parser:"):
		var data struct {
			Message ParseMessage `yaml:"message"`
		}
		err = node.Decode(&data)
		if err != nil {
			return err
		}
		su.ParseMessage = data.Message
	case data.Type == "wrapper":
		var data struct {
			Message WrapMessage `yaml:"message"`
		}
		err = node.Decode(&data)
		if err != nil {
			return err
		}
		su.WrapMessage = data.Message
	}

	su.Type = data.Type
	su.Trigger = data.Trigger
	return nil
}

// FindWithPrefix finds a template with a matching command prefix.
func FindWithPrefix(tpls []Template, cmd string) (Template, bool) {
	for idx := range tpls {
		item := tpls[idx]
		if item.Trigger.Command == "" {
			continue
		}
		if strings.HasPrefix(cmd, item.Trigger.Command) {
			return item, true
		}
	}

	return Template{}, false
}
