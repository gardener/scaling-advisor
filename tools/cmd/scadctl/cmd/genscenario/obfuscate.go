// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package genscenario

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/gardener/scaling-advisor/api/planner"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	removePrefixes = []string{
		"beta.", "failure-domain.beta.", "node.alpha.kubernetes.io", "checksum/",
		"node-agent.gardener.cloud", "worker.gardener.cloud/gardener-node-agent-secret-name",
		"resources.gardener.cloud", "shoot.gardener.cloud", "node.gardener.cloud/machine-name",
		"node.machine.sapcloud.io/last-applied-anno-labels-taints", "cni.",
		"controller-revision-hash", "gardener.cloud/role", "networking.gardener.cloud/", "node.gardener.cloud/critical-component",
		"pod-template-generation", "reference.resources.gardener.cloud/", "csi.volume.kubernetes.io",
		"volumes.kubernetes.io/controller-managed-attach-detach",
		"topology.ebs", "worker.gardener", "worker.garden", "topology.k8s.aws",
	}
)

func sanitizeDeleteFunc(k, _ string) bool {
	for _, prefix := range removePrefixes {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

func obfuscateMetadata(snap *planner.ClusterSnapshot) {
	// Collect all label keys referenced by scheduling constraints
	// (NodeSelector, Affinity, TopologySpreadConstraints, Tolerations).
	schedulingKeys := collectSchedulingRelevantKeys(snap)

	// Build obfuscation map for scheduling-relevant keys that have removable
	// prefixes. These keys are renamed consistently across the entire snapshot
	// instead of being deleted, so that scheduling semantics are preserved.
	keyMap := buildSchedulingKeyMap(&schedulingKeys)

	// Build consistent node name mapping.
	nodeNameMap := make(map[string]string, len(snap.Nodes))
	for i, node := range snap.Nodes {
		nodeNameMap[node.Name] = "node-" + strconv.Itoa(i)
	}

	// Build consistent owner reference name mapping so that pods sharing
	// the same owner (e.g. ReplicaSet) get the same obfuscated owner name.
	ownerNameMap := make(map[string]string)
	ownerID := 0
	for _, pod := range snap.Pods {
		for _, owner := range pod.OwnerReferences {
			if _, exists := ownerNameMap[owner.Name]; !exists {
				ownerNameMap[owner.Name] = "owner-" + strconv.Itoa(ownerID)
				ownerID++
			}
		}
	}

	// Obfuscate nodes.
	for i := range snap.Nodes {
		node := &snap.Nodes[i]
		node.Name = nodeNameMap[node.Name]
		obfuscateStringMap(node.Labels, keyMap)
		obfuscateTaints(node.Taints, keyMap)
		maps.DeleteFunc(node.Annotations, sanitizeDeleteFunc)
	}

	// Obfuscate pods.
	for i := range snap.Pods {
		pod := &snap.Pods[i]
		suffix := ""
		if len(pod.OwnerReferences) >= 1 && pod.OwnerReferences[0].Kind == "DaemonSet" {
			suffix = "-ds"
		}
		pod.Name = "pod-" + strconv.Itoa(i) + suffix
		pod.GenerateName = ""

		if newName, ok := nodeNameMap[pod.NodeName]; ok {
			pod.NodeName = newName
		}
		for j := range pod.OwnerReferences {
			if newName, ok := ownerNameMap[pod.OwnerReferences[j].Name]; ok {
				pod.OwnerReferences[j].Name = newName
			}
		}

		maps.DeleteFunc(pod.Annotations, sanitizeDeleteFunc)
		obfuscateStringMap(pod.Labels, keyMap)
		obfuscateStringMap(pod.NodeSelector, keyMap)
		obfuscateAffinityRules(pod.Affinity, keyMap)
		obfuscateTSCRules(pod.TopologySpreadConstraints, keyMap)
		obfuscateTolerations(pod.Tolerations, keyMap)
	}

	// Obfuscate priority classes.
	for i := range snap.PriorityClasses {
		obfuscateStringMap(snap.PriorityClasses[i].Labels, keyMap)
		maps.DeleteFunc(snap.PriorityClasses[i].Annotations, sanitizeDeleteFunc)
	}
}

// ---------------------------------------------------------------------------------
// Scheduling key collection
// ---------------------------------------------------------------------------------

// collectSchedulingRelevantKeys scans all pods and returns a set of label keys
// that are referenced by scheduling constraints. Removing or renaming these
// keys without updating every reference would break scheduling.
func collectSchedulingRelevantKeys(snap *planner.ClusterSnapshot) sets.Set[string] {
	keys := sets.New[string]()
	for _, pod := range snap.Pods {
		keys.Insert(slices.Collect(maps.Keys(pod.NodeSelector))...)
		for _, t := range pod.Tolerations {
			if t.Key != "" {
				keys.Insert(t.Key)
			}
		}
		collectAffinityKeys(pod.Affinity, &keys)
		for _, tsc := range pod.TopologySpreadConstraints {
			if tsc.TopologyKey != "" {
				keys.Insert(tsc.TopologyKey)
			}
			collectLabelSelectorKeys(tsc.LabelSelector, &keys)
			keys.Insert(tsc.MatchLabelKeys...)
		}
	}
	return keys
}

func collectAffinityKeys(affinity *corev1.Affinity, keys *sets.Set[string]) {
	if affinity == nil {
		return
	}
	if na := affinity.NodeAffinity; na != nil {
		if req := na.RequiredDuringSchedulingIgnoredDuringExecution; req != nil {
			for _, term := range req.NodeSelectorTerms {
				collectNodeSelectorRequirementKeys(term.MatchExpressions, keys)
				collectNodeSelectorRequirementKeys(term.MatchFields, keys)
			}
		}
		for _, pref := range na.PreferredDuringSchedulingIgnoredDuringExecution {
			collectNodeSelectorRequirementKeys(pref.Preference.MatchExpressions, keys)
			collectNodeSelectorRequirementKeys(pref.Preference.MatchFields, keys)
		}
	}
	if pa := affinity.PodAffinity; pa != nil {
		collectPodAffinityTermKeys(pa.RequiredDuringSchedulingIgnoredDuringExecution, keys)
		collectWeightedPodAffinityTermKeys(pa.PreferredDuringSchedulingIgnoredDuringExecution, keys)
	}
	if paa := affinity.PodAntiAffinity; paa != nil {
		collectPodAffinityTermKeys(paa.RequiredDuringSchedulingIgnoredDuringExecution, keys)
		collectWeightedPodAffinityTermKeys(paa.PreferredDuringSchedulingIgnoredDuringExecution, keys)
	}
}

func collectNodeSelectorRequirementKeys(reqs []corev1.NodeSelectorRequirement, keys *sets.Set[string]) {
	for _, req := range reqs {
		keys.Insert(req.Key)
	}
}

func collectPodAffinityTermKeys(terms []corev1.PodAffinityTerm, keys *sets.Set[string]) {
	for _, term := range terms {
		if term.TopologyKey != "" {
			keys.Insert(term.TopologyKey)
		}
		collectLabelSelectorKeys(term.LabelSelector, keys)
		collectLabelSelectorKeys(term.NamespaceSelector, keys)
	}
}

func collectWeightedPodAffinityTermKeys(terms []corev1.WeightedPodAffinityTerm, keys *sets.Set[string]) {
	for _, term := range terms {
		if term.PodAffinityTerm.TopologyKey != "" {
			keys.Insert(term.PodAffinityTerm.TopologyKey)
		}
		collectLabelSelectorKeys(term.PodAffinityTerm.LabelSelector, keys)
		collectLabelSelectorKeys(term.PodAffinityTerm.NamespaceSelector, keys)
	}
}

func collectLabelSelectorKeys(selector *metav1.LabelSelector, keys *sets.Set[string]) {
	if selector == nil {
		return
	}
	keys.Insert(slices.Collect(maps.Keys(selector.MatchLabels))...)
	for _, expr := range selector.MatchExpressions {
		keys.Insert(expr.Key)
	}
}

// ---------------------------------------------------------------------------------
// Obfuscation map construction
// ---------------------------------------------------------------------------------

// buildSchedulingKeyMap identifies scheduling-relevant keys that have removable
// prefixes and returns a mapping from the original key to an obfuscated key
// (e.g. "topology.ebs.csi.aws.com/zone" -> "sched-key-0").
func buildSchedulingKeyMap(schedulingKeys *sets.Set[string]) map[string]string {
	keyMap := make(map[string]string)
	keyID := 0
	for key := range *schedulingKeys {
		if sanitizeDeleteFunc(key, "") {
			keyMap[key] = "sched-key-" + strconv.Itoa(keyID)
			keyID++
		}
	}
	return keyMap
}

// ---------------------------------------------------------------------------------
// Obfuscation application
// ---------------------------------------------------------------------------------

// obfuscateStringMap processes a string map (labels, NodeSelector, MatchLabels):
//   - keys in keyMap are renamed; these are scheduling-relevant keys that have
//     a removable prefix.
//   - keys with a removable prefix that are NOT in keyMap (i.e. not scheduling-
//     relevant) are deleted.
//   - all other keys are left as-is.
func obfuscateStringMap(m map[string]string, keyMap map[string]string) {
	if len(m) == 0 {
		return
	}
	type renamed struct {
		newKey, val string
	}
	renames := make(map[string]renamed) // oldKey -> renamed
	var deletes []string

	for k, v := range m {
		if newKey, ok := keyMap[k]; ok {
			renames[k] = renamed{newKey, v}
		} else if sanitizeDeleteFunc(k, "") {
			deletes = append(deletes, k)
		}
	}
	for _, d := range deletes {
		delete(m, d)
	}
	for oldKey, r := range renames {
		delete(m, oldKey)
		m[r.newKey] = r.val
	}
}

func obfuscateAffinityRules(affinity *corev1.Affinity, keyMap map[string]string) {
	if affinity == nil {
		return
	}
	if na := affinity.NodeAffinity; na != nil {
		if req := na.RequiredDuringSchedulingIgnoredDuringExecution; req != nil {
			for i := range req.NodeSelectorTerms {
				obfuscateNodeSelectorRequirements(req.NodeSelectorTerms[i].MatchExpressions, keyMap)
				obfuscateNodeSelectorRequirements(req.NodeSelectorTerms[i].MatchFields, keyMap)
			}
		}
		for i := range na.PreferredDuringSchedulingIgnoredDuringExecution {
			obfuscateNodeSelectorRequirements(na.PreferredDuringSchedulingIgnoredDuringExecution[i].Preference.MatchExpressions, keyMap)
			obfuscateNodeSelectorRequirements(na.PreferredDuringSchedulingIgnoredDuringExecution[i].Preference.MatchFields, keyMap)
		}
	}
	if pa := affinity.PodAffinity; pa != nil {
		obfuscatePodAffinityTerms(pa.RequiredDuringSchedulingIgnoredDuringExecution, keyMap)
		obfuscateWeightedPodAffinityTerms(pa.PreferredDuringSchedulingIgnoredDuringExecution, keyMap)
	}
	if paa := affinity.PodAntiAffinity; paa != nil {
		obfuscatePodAffinityTerms(paa.RequiredDuringSchedulingIgnoredDuringExecution, keyMap)
		obfuscateWeightedPodAffinityTerms(paa.PreferredDuringSchedulingIgnoredDuringExecution, keyMap)
	}
}

func obfuscateNodeSelectorRequirements(reqs []corev1.NodeSelectorRequirement, keyMap map[string]string) {
	for i := range reqs {
		if newKey, ok := keyMap[reqs[i].Key]; ok {
			reqs[i].Key = newKey
		}
	}
}

func obfuscatePodAffinityTerms(terms []corev1.PodAffinityTerm, keyMap map[string]string) {
	for i := range terms {
		if newKey, ok := keyMap[terms[i].TopologyKey]; ok {
			terms[i].TopologyKey = newKey
		}
		obfuscateLabelSelector(terms[i].LabelSelector, keyMap)
		obfuscateLabelSelector(terms[i].NamespaceSelector, keyMap)
	}
}

func obfuscateWeightedPodAffinityTerms(terms []corev1.WeightedPodAffinityTerm, keyMap map[string]string) {
	for i := range terms {
		if newKey, ok := keyMap[terms[i].PodAffinityTerm.TopologyKey]; ok {
			terms[i].PodAffinityTerm.TopologyKey = newKey
		}
		obfuscateLabelSelector(terms[i].PodAffinityTerm.LabelSelector, keyMap)
		obfuscateLabelSelector(terms[i].PodAffinityTerm.NamespaceSelector, keyMap)
	}
}

func obfuscateLabelSelector(sel *metav1.LabelSelector, keyMap map[string]string) {
	if sel == nil {
		return
	}
	obfuscateStringMap(sel.MatchLabels, keyMap)
	for i := range sel.MatchExpressions {
		if newKey, ok := keyMap[sel.MatchExpressions[i].Key]; ok {
			sel.MatchExpressions[i].Key = newKey
		}
	}
}

func obfuscateTSCRules(tscs []corev1.TopologySpreadConstraint, keyMap map[string]string) {
	for i := range tscs {
		if newKey, ok := keyMap[tscs[i].TopologyKey]; ok {
			tscs[i].TopologyKey = newKey
		}
		obfuscateLabelSelector(tscs[i].LabelSelector, keyMap)
		for j, k := range tscs[i].MatchLabelKeys {
			if newKey, ok := keyMap[k]; ok {
				tscs[i].MatchLabelKeys[j] = newKey
			}
		}
	}
}

func obfuscateTolerations(tolerations []corev1.Toleration, keyMap map[string]string) {
	for i := range tolerations {
		if newKey, ok := keyMap[tolerations[i].Key]; ok {
			tolerations[i].Key = newKey
		}
	}
}

func obfuscateTaints(taints []corev1.Taint, keyMap map[string]string) {
	for i := range taints {
		if newKey, ok := keyMap[taints[i].Key]; ok {
			taints[i].Key = newKey
		}
	}
}
