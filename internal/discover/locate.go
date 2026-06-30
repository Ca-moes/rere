package discover

import (
	"bytes"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/kio"
)

// buildIndex walks root for YAML manifests (honoring include/exclude globs) and
// indexes each document by (kind, name, namespace). Doc indices count every
// document in a file so they line up with yamledit reading the same file.
func buildIndex(root string, include, exclude []string) (map[metaKey][]Location, error) {
	index := make(map[metaKey][]Location)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if !included(rel, include) || matchAny(rel, exclude) {
			return nil
		}
		metas, readErr := readManifestMetas(path)
		if readErr != nil {
			// A single unparseable file (e.g. a Helm chart template with Go
			// template directives) must not abort the whole scan — skip it,
			// consistent with how non-resource documents are skipped below.
			slog.Debug("discover: skipping unparseable file", "path", path, "err", readErr)
			return nil
		}
		for _, m := range metas {
			key := metaKey{kind: m.kind, name: m.name, namespace: m.namespace}
			index[key] = append(index[key], Location{File: path, DocIndex: m.docIndex})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return index, nil
}

func isYAML(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func included(rel string, include []string) bool {
	return len(include) == 0 || matchAny(rel, include)
}

// matchAny reports whether rel matches any glob, treating a pattern as a
// directory prefix when rel sits beneath it (e.g. "overlays/staging").
func matchAny(rel string, patterns []string) bool {
	for _, p := range patterns {
		p = filepath.ToSlash(p)
		if ok, _ := filepath.Match(p, rel); ok {
			return true
		}
		if strings.HasPrefix(rel, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

type manifestMeta struct {
	kind      string
	name      string
	namespace string
	docIndex  int
}

// readManifestMetas parses every document in a file and returns the kind / name
// / namespace of those that look like Kubernetes resources. Non-resource docs
// (e.g. Helm values) and empty docs are skipped, but their position is still
// counted so docIndex matches the editor's view of the file.
func readManifestMetas(path string) ([]manifestMeta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	nodes, err := (&kio.ByteReader{Reader: bytes.NewReader(b), OmitReaderAnnotations: true}).Read()
	if err != nil {
		return nil, err
	}
	var metas []manifestMeta
	for i, n := range nodes {
		m, err := n.GetMeta()
		if err != nil {
			continue // not a structured resource document
		}
		if m.Kind == "" || m.Name == "" {
			continue
		}
		metas = append(metas, manifestMeta{
			kind:      m.Kind,
			name:      m.Name,
			namespace: m.Namespace,
			docIndex:  i,
		})
	}
	return metas, nil
}
