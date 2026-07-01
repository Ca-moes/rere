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
		name, component, ok := matchWorkload(cm.Match, cm.nameRE, t)
		if !ok {
			continue
		}
		out := t
		out.Kind = cm.Kind
		out.Name = name
		out.Container = component
		return out, true
	}
	return t, false
}

// matchWorkload applies one match rule to a recommender target, returning the
// recovered owning-resource name and the selected component. Shared by tier-2 CR
// translation and tier-3 HelmRelease translation.
//
// When a rule enumerates containers, only the ones it names translate: an
// unlisted container (e.g. a CNPG pod's monitoring sidecar) is not represented
// by this resource's block, and for a single-component map it would otherwise
// resolve to the same resources path as a mapped container and clobber its edit.
func matchWorkload(m MatchRule, nameRE *regexp.Regexp, t adapter.Target) (name, component string, ok bool) {
	if m.WorkloadKind != "" && m.WorkloadKind != t.Kind {
		return "", "", false
	}
	component, mapped := m.ContainerToComponent[t.Container]
	if len(m.ContainerToComponent) > 0 && !mapped {
		return "", "", false
	}
	name, ok = recoverName(m, nameRE, t.Name)
	if !ok {
		return "", "", false
	}
	return name, component, true
}

// recoverName extracts the owning resource's name from a generated-workload name
// using the rule's NamePattern (first capture group) or NameSuffix (trimmed);
// with neither set the name equals the workload name. nameRE is the precompiled
// NamePattern (cached by MergedMaps / MergedChartMaps); a nil nameRE falls back
// to a lazy compile for a directly-constructed rule.
func recoverName(m MatchRule, nameRE *regexp.Regexp, workloadName string) (string, bool) {
	switch {
	case m.NamePattern != "":
		re := nameRE
		if re == nil {
			var err error
			if re, err = regexp.Compile(m.NamePattern); err != nil {
				return "", false // Validate rejects bad patterns before we get here
			}
		}
		sub := re.FindStringSubmatch(workloadName)
		if len(sub) < 2 {
			return "", false
		}
		return sub[1], true
	case m.NameSuffix != "":
		if !strings.HasSuffix(workloadName, m.NameSuffix) {
			return "", false
		}
		return strings.TrimSuffix(workloadName, m.NameSuffix), true
	default:
		return workloadName, true
	}
}
