package config

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func configFieldNode(node *yaml.Node, field string) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	for _, part := range strings.FieldsFunc(field, func(r rune) bool { return r == '.' || r == '[' || r == ']' }) {
		for node.Kind == yaml.AliasNode {
			node = node.Alias
		}
		var next *yaml.Node
		switch node.Kind {
		case yaml.MappingNode:
			next = configMappingValue(node, part)
		case yaml.SequenceNode:
			index, err := strconv.Atoi(part)
			if err == nil && index >= 0 && index < len(node.Content) {
				next = node.Content[index]
			}
		}
		if next == nil {
			break
		}
		node = next
	}
	return node
}

func configMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind == yaml.AliasNode {
		return configMappingValue(node.Alias, key)
	}
	if node.Kind == yaml.SequenceNode {
		for _, item := range node.Content {
			if value := configMappingValue(item, key); value != nil {
				return value
			}
		}
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Tag == "!!merge" {
			return configMappingValue(node.Content[i+1], key)
		}
	}
	return nil
}
