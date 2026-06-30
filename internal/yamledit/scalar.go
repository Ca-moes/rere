package yamledit

import (
	"io"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/Ca-moes/rere/internal/adapter"
)

// SelectDoc returns the first document in src whose kind+name match, or nil if
// none match. The returned RNode is used by callers to drive FieldMapper
// Supports/Resolve and ReadCurrentAt without re-parsing.
func SelectDoc(src []byte, kind, name string) (*yaml.RNode, error) {
	nodes, err := readNodes(src)
	if err != nil {
		return nil, err
	}
	for _, doc := range nodes {
		meta, err := doc.GetMeta()
		if err != nil {
			continue
		}
		if meta.Kind == kind && meta.Name == name {
			return doc, nil
		}
	}
	return nil, nil
}

// ReadCurrentAt reads the current requests/limits at the paths a FieldMapper
// resolves for each (section, resource) cell, so the policy engine can compare
// against recommendations regardless of where the resources live. Resolution
// errors propagate; a missing or unparseable scalar yields a nil quantity (the
// cell is simply absent).
func ReadCurrentAt(root *yaml.RNode, resolve func(section, resource string) ([]string, error)) (adapter.Recommended, error) {
	read := func(section, res string) (*resource.Quantity, error) {
		path, err := resolve(section, res)
		if err != nil {
			return nil, err
		}
		return readScalarQuantity(root, path), nil
	}
	reqCPU, err := read("requests", "cpu")
	if err != nil {
		return adapter.Recommended{}, err
	}
	reqMem, err := read("requests", "memory")
	if err != nil {
		return adapter.Recommended{}, err
	}
	limCPU, err := read("limits", "cpu")
	if err != nil {
		return adapter.Recommended{}, err
	}
	limMem, err := read("limits", "memory")
	if err != nil {
		return adapter.Recommended{}, err
	}
	return adapter.Recommended{
		Requests: adapter.ResourceValues{CPU: reqCPU, Mem: reqMem},
		Limits:   adapter.ResourceValues{CPU: limCPU, Mem: limMem},
	}, nil
}

// ReadCurrent reads the current requests/limits of a tier-1 container. A missing
// doc or container yields an empty Recommended (no error). It is a thin shim
// over ReadCurrentAt using the tier-1 PodSpec path, kept for callers/tests that
// don't hold a FieldMapper.
func ReadCurrent(in io.Reader, kind, name, container string) (adapter.Recommended, error) {
	src, err := io.ReadAll(in)
	if err != nil {
		return adapter.Recommended{}, err
	}
	root, err := SelectDoc(src, kind, name)
	if err != nil || root == nil {
		return adapter.Recommended{}, err
	}
	base := append(podSpecPath(kind), "containers", "[name="+container+"]", "resources")
	return ReadCurrentAt(root, func(section, res string) ([]string, error) {
		return append(append([]string{}, base...), section, res), nil
	})
}

func readScalarQuantity(root *yaml.RNode, path []string) *resource.Quantity {
	n, err := root.Pipe(yaml.Lookup(path...))
	if err != nil || n == nil {
		return nil
	}
	q, err := resource.ParseQuantity(yaml.GetValue(n))
	if err != nil {
		return nil
	}
	return &q
}
