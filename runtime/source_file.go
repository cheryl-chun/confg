package runtime

import (
	"fmt"

	"github.com/cheryl-chun/confgen/internal/parser"
	"github.com/cheryl-chun/confgen/internal/tree"
)

// FileSource 文件配置源
type FileSource struct {
	Path string
}

// Load 实现 Source 接口
func (s *FileSource) Load(configTree *tree.ConfigTree) error {
	tempTree, err := s.parseTree()
	if err != nil {
		return err
	}
	configTree.ReplaceSource(tempTree, tree.SourceFile)
	return nil
}

// Priority 返回文件配置源的优先级
func (s *FileSource) Priority() tree.SourceType {
	return tree.SourceFile
}

func (s *FileSource) parseTree() (*tree.ConfigTree, error) {
	tempTree, err := parser.ParseToTree(s.Path, tree.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file %s: %w", s.Path, err)
	}
	return tempTree, nil
}

// mergeTrees 将 src tree 合并到 dst tree
func (s *FileSource) mergeTrees(dst, src *tree.ConfigTree) error {
	// 简化实现：递归遍历 src 的根节点，将所有值复制到 dst
	return s.mergeNode(dst, "", src.Root)
}

// mergeNode 递归合并节点
func (s *FileSource) mergeNode(dst *tree.ConfigTree, prefix string, node *tree.ConfigNode) error {
	if node == nil {
		return nil
	}

	// 构建当前节点的完整路径
	path := prefix
	if prefix != "" && node.Key != "root" {
		path = prefix + "." + node.Key
	} else if node.Key != "root" {
		path = node.Key
	}

	// 如果节点有值，设置到 dst
	if node.HasValue() {
		if path != "" {
			dst.Set(path, node.GetValue(), tree.SourceFile, node.Type)
		}
	}

	// 递归处理子节点
	for _, child := range node.Children {
		if err := s.mergeNode(dst, path, child); err != nil {
			return err
		}
	}

	// 递归处理数组元素
	for _, item := range node.Items {
		if err := s.mergeNode(dst, path, item); err != nil {
			return err
		}
	}

	return nil
}
