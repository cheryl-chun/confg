package parser

import (
	"strconv"

	"github.com/cheryl-chun/confgen/internal/tree"
)

// ParseToTree parses a configuration file and returns a ConfigTree
//
// Logic:
// 1. Use the existing parser to read the file (YAML/JSON)
// 2. Build a tree.ConfigTree directly instead of parser.ConfigNode
// 3. Set the source type for all values
//
// Example:
//
//	configTree, err := parser.ParseToTree("config.yaml", tree.SourceFile)
//	value, _ := configTree.GetValue("server.host")
func ParseToTree(path string, source tree.SourceType) (*tree.ConfigTree, error) {
	// Step 1: Parse the file using the factory
	factory := GetFactory()
	parser, err := factory.GetParserByFilePath(path)
	if err != nil {
		return nil, err
	}

	result, err := parser.ParseFile(path)
	if err != nil {
		return nil, err
	}

	// Step 2: Create a new ConfigTree
	configTree := tree.NewConfigTree()

	// Step 3: Convert parser.ConfigNode to tree.ConfigTree
	// This is where the magic happens!
	convertToTree(configTree, "", result.Root, source)

	return configTree, nil
}

// convertToTree recursively converts parser.ConfigNode to tree.ConfigTree
//
// Logic explanation:
// - Parser's ConfigNode is a temporary intermediate representation
// - Tree's ConfigTree is the final storage structure with multi-source support
// - We need to traverse the parser tree and build the tree structure
//
// Parameters:
// - configTree: the target tree we're building
// - parentPath: current path in dot notation (e.g., "server.database")
// - node: current parser.ConfigNode we're processing
// - source: which source this configuration comes from (File/Env/Remote/etc)
func convertToTree(configTree *tree.ConfigTree, parentPath string, node *ConfigNode, source tree.SourceType) {
	if node == nil {
		return
	}

	// Handle different node types
	switch {
	case node.IsObject():
		// Object node: recursively process children
		// Example: server { host, port, timeout }
		for key, child := range node.Children {
			// Build the full path
			// e.g., parentPath="server", key="host" → fullPath="server.host"
			fullPath := key
			if parentPath != "" {
				fullPath = parentPath + "." + key
			}

			// Recursively process child node
			convertToTree(configTree, fullPath, child, source)
		}

	case node.IsArray():
		// Array node: process each item
		// We keep the full array value, and also flatten items into index paths
		// such as "features.0" and "servers.0.host" for fast indexed lookup.

		// Extract array values
		arrayValues := make([]any, len(node.Items))
		for i, item := range node.Items {
			arrayValues[i] = extractValue(item)
			if parentPath != "" {
				indexPath := parentPath + "." + strconv.Itoa(i)
				convertArrayItemToTree(configTree, indexPath, item, source)
			}
		}

		// Set the array in the tree
		if parentPath != "" {
			configTree.Set(parentPath, arrayValues, source, tree.TypeArray)
		}

	case node.IsPrimitive():
		// Primitive value: directly set in tree
		// Example: "localhost", 8080, true, etc.
		if parentPath != "" {
			// Convert parser.ValueType to tree.ValueType
			treeType := convertValueType(node.Type)
			configTree.Set(parentPath, node.Value, source, treeType)
		}
	}
}

// convertArrayItemToTree flattens an array element to indexed paths.
// Example:
// - primitive: features.0 = "ssl"
// - object: servers.0.host = "server1.com"
// - nested array: matrix.0.1 = 42
func convertArrayItemToTree(configTree *tree.ConfigTree, indexPath string, node *ConfigNode, source tree.SourceType) {
	if node == nil || indexPath == "" {
		return
	}

	switch {
	case node.IsPrimitive():
		configTree.Set(indexPath, node.Value, source, convertValueType(node.Type))

	case node.IsObject():
		for key, child := range node.Children {
			childPath := indexPath + "." + key
			convertToTree(configTree, childPath, child, source)
		}

	case node.IsArray():
		nestedValues := make([]any, len(node.Items))
		for i, item := range node.Items {
			nestedValues[i] = extractValue(item)
			nestedPath := indexPath + "." + strconv.Itoa(i)
			convertArrayItemToTree(configTree, nestedPath, item, source)
		}
		configTree.Set(indexPath, nestedValues, source, tree.TypeArray)
	}
}

// extractValue extracts the actual value from a parser.ConfigNode
// This is used for array elements
func extractValue(node *ConfigNode) any {
	if node == nil {
		return nil
	}

	switch {
	case node.IsPrimitive():
		return node.Value
	case node.IsArray():
		// Nested array
		values := make([]any, len(node.Items))
		for i, item := range node.Items {
			values[i] = extractValue(item)
		}
		return values
	case node.IsObject():
		// Object in array - convert to map
		obj := make(map[string]any)
		for key, child := range node.Children {
			obj[key] = extractValue(child)
		}
		return obj
	}

	return nil
}

// convertValueType converts parser.ValueType to tree.ValueType
// They are separate types but have the same values (for now)
//
// Why separate types?
// - parser package: temporary, for file parsing only
// - tree package: permanent, core data structure
// - Decoupling allows independent evolution
func convertValueType(parserType ValueType) tree.ValueType {
	switch parserType {
	case TypeString:
		return tree.TypeString
	case TypeInt:
		return tree.TypeInt
	case TypeFloat:
		return tree.TypeFloat
	case TypeBool:
		return tree.TypeBool
	case TypeArray:
		return tree.TypeArray
	case TypeObject:
		return tree.TypeObject
	case TypeNull:
		return tree.TypeNull
	default:
		return tree.TypeNull
	}
}
