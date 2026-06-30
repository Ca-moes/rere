package yamledit

import (
	"io"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/Ca-moes/rere/internal/adapter"
)

// ReadCurrent reads the current requests/limits of a container from a manifest,
// for the policy engine to compare against recommendations. A missing doc or
// container yields an empty Recommended (no error).
func ReadCurrent(in io.Reader, kind, name, container string) (adapter.Recommended, error) {
	src, err := io.ReadAll(in)
	if err != nil {
		return adapter.Recommended{}, err
	}
	nodes, err := readNodes(src)
	if err != nil {
		return adapter.Recommended{}, err
	}
	for _, doc := range nodes {
		meta, err := doc.GetMeta()
		if err != nil || meta.Kind != kind || meta.Name != name {
			continue
		}
		path := append(podSpecPath(kind), "containers", "[name="+container+"]")
		c, err := doc.Pipe(yaml.Lookup(path...))
		if err != nil {
			return adapter.Recommended{}, err
		}
		if c == nil {
			return adapter.Recommended{}, nil
		}
		return adapter.Recommended{
			Requests: readValues(c, "requests"),
			Limits:   readValues(c, "limits"),
		}, nil
	}
	return adapter.Recommended{}, nil
}

func readValues(container *yaml.RNode, section string) adapter.ResourceValues {
	return adapter.ResourceValues{
		CPU: readQuantity(container, section, "cpu"),
		Mem: readQuantity(container, section, "memory"),
	}
}

func readQuantity(container *yaml.RNode, section, res string) *resource.Quantity {
	n, err := container.Pipe(yaml.Lookup("resources", section, res))
	if err != nil || n == nil {
		return nil
	}
	q, err := resource.ParseQuantity(yaml.GetValue(n))
	if err != nil {
		return nil
	}
	return &q
}
