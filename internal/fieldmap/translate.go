package fieldmap

import (
	"regexp"
	"strings"

	"github.com/Ca-moes/rere/internal/adapter"
)

// TranslateTarget rewrites a recommender target that names an operator-generated
// workload (e.g. the Deployment "otel-collector", or a CNPG instance Pod
// "mycluster-1") into the owning CR's identity ({CR kind, CR name, component}),
// so repo-scan discovery finds the CR manifest and the tier-2 mapper resolves
// the right subtree. It returns (target, false) unchanged when no map matches —
// the target is a raw workload that tier-1 handles. Run this before grouping so
// several instance pods collapse into one CR (one PR).
func TranslateTarget(t adapter.Target, maps MapConfig) (adapter.Target, bool) {
	for i := range maps.Maps {
		cm := &maps.Maps[i]
		if cm.Match.WorkloadKind != "" && cm.Match.WorkloadKind != t.Kind {
			continue
		}
		component, mapped := cm.Match.ContainerToComponent[t.Container]
		// When a map enumerates containers, translate only the ones it names: an
		// unlisted container (e.g. a CNPG pod's monitoring sidecar) is not
		// represented by this CR's resources, and for a single-component map it
		// would otherwise resolve to the same spec.resources path as the mapped
		// container and clobber its edit.
		if len(cm.Match.ContainerToComponent) > 0 && !mapped {
			continue
		}
		crName, ok := recoverCRName(cm, t.Name)
		if !ok {
			continue
		}
		out := t
		out.Kind = cm.Kind
		out.Name = crName
		out.Container = component
		return out, true
	}
	return t, false
}

// recoverCRName extracts the CR name from a generated-workload name using the
// rule's NamePattern (first capture group) or NameSuffix (trimmed); with neither
// set the CR name equals the workload name. The NamePattern regexp is the one
// MergedMaps cached; for a directly-constructed map it is compiled lazily.
func recoverCRName(cm *CRMap, workloadName string) (string, bool) {
	switch {
	case cm.Match.NamePattern != "":
		re := cm.nameRE
		if re == nil {
			var err error
			if re, err = regexp.Compile(cm.Match.NamePattern); err != nil {
				return "", false // Validate rejects bad patterns before we get here
			}
		}
		sub := re.FindStringSubmatch(workloadName)
		if len(sub) < 2 {
			return "", false
		}
		return sub[1], true
	case cm.Match.NameSuffix != "":
		if !strings.HasSuffix(workloadName, cm.Match.NameSuffix) {
			return "", false
		}
		return strings.TrimSuffix(workloadName, cm.Match.NameSuffix), true
	default:
		return workloadName, true
	}
}
