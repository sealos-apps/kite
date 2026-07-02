package helmutil

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/zxh326/kite/pkg/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	release "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const (
	chartSourceAnnotation         = "kite.sh/chart-source"
	chartRepositoryNameAnnotation = "kite.sh/chart-repository-name"
	chartURLAnnotation            = "kite.sh/chart-url"
	chartNameAnnotation           = "kite.sh/chart-name"
	chartVersionAnnotation        = "kite.sh/chart-version"
)

type ChartProvenance struct {
	Source         string
	RepositoryName string
	ChartName      string
	Version        string
	URL            string
}

type OfflineImagePolicy struct {
	Enabled  bool
	Registry string
	Enforce  bool
}

type ImageCheckResult struct {
	Enabled        bool     `json:"enabled"`
	Registry       string   `json:"registry,omitempty"`
	AllImages      []string `json:"allImages,omitempty"`
	ExternalImages []string `json:"externalImages,omitempty"`
	InjectedValues bool     `json:"injectedValues,omitempty"`
}

func OfflineImagePolicyForSource(source string) OfflineImagePolicy {
	if source != ChartSourceOCI {
		return OfflineImagePolicy{}
	}
	registry := strings.TrimSpace(common.HelmOfflineImagesRegistry)
	if !common.HelmOfflineImagesEnabled || registry == "" {
		return OfflineImagePolicy{}
	}
	return OfflineImagePolicy{
		Enabled:  true,
		Registry: registry,
		Enforce:  common.HelmOfflineImagesEnforce,
	}
}

func PrepareReleaseValues(values map[string]interface{}, source string) (map[string]interface{}, OfflineImagePolicy, bool) {
	policy := OfflineImagePolicyForSource(source)
	prepared, injected := ApplyOfflineImagePolicy(values, policy)
	return prepared, policy, injected
}

func AnnotateChartSource(ch *chart.Chart, provenance ChartProvenance) {
	if ch == nil || ch.Metadata == nil {
		return
	}
	if ch.Metadata.Annotations == nil {
		ch.Metadata.Annotations = map[string]string{}
	}
	setAnnotation := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			ch.Metadata.Annotations[key] = value
		}
	}
	setAnnotation(chartSourceAnnotation, provenance.Source)
	setAnnotation(chartRepositoryNameAnnotation, provenance.RepositoryName)
	setAnnotation(chartURLAnnotation, provenance.URL)
	setAnnotation(chartNameAnnotation, provenance.ChartName)
	setAnnotation(chartVersionAnnotation, provenance.Version)
}

func ReleaseChartSource(rel *release.Release) string {
	return ReleaseChartProvenance(rel).Source
}

func ReleaseChartProvenance(rel *release.Release) ChartProvenance {
	if rel == nil || rel.Chart == nil || rel.Chart.Metadata == nil {
		return ChartProvenance{}
	}
	annotations := rel.Chart.Metadata.Annotations
	return ChartProvenance{
		Source:         strings.TrimSpace(annotations[chartSourceAnnotation]),
		RepositoryName: strings.TrimSpace(annotations[chartRepositoryNameAnnotation]),
		ChartName:      strings.TrimSpace(firstNonEmpty(annotations[chartNameAnnotation], rel.Chart.Metadata.Name)),
		Version:        strings.TrimSpace(firstNonEmpty(annotations[chartVersionAnnotation], rel.Chart.Metadata.Version)),
		URL:            strings.TrimSpace(annotations[chartURLAnnotation]),
	}
}

func ApplyOfflineImagePolicy(values map[string]interface{}, policy OfflineImagePolicy) (map[string]interface{}, bool) {
	if !policy.Enabled || policy.Registry == "" {
		return values, false
	}
	out := cloneValuesMap(values)
	global, _ := out["global"].(map[string]interface{})
	if global == nil {
		global = map[string]interface{}{}
		out["global"] = global
	}
	injected := false
	if existing, ok := global["imageRegistry"].(string); !ok || strings.TrimSpace(existing) == "" {
		global["imageRegistry"] = policy.Registry
		injected = true
	}
	security, _ := global["security"].(map[string]interface{})
	if security == nil {
		security = map[string]interface{}{}
		global["security"] = security
	}
	if _, ok := security["allowInsecureImages"]; !ok {
		security["allowInsecureImages"] = true
		injected = true
	}
	return out, injected
}

func CheckReleaseImages(rel *release.Release, policy OfflineImagePolicy, injectedValues bool) (ImageCheckResult, error) {
	result := ImageCheckResult{
		Enabled:        policy.Enabled,
		Registry:       policy.Registry,
		InjectedValues: injectedValues,
	}
	if rel == nil || !policy.Enabled {
		return result, nil
	}
	images := ExtractManifestImages(rel.Manifest)
	for _, hook := range rel.Hooks {
		images = append(images, ExtractManifestImages(hook.Manifest)...)
	}
	result.AllImages = uniqueSortedStrings(images)
	for _, image := range result.AllImages {
		if !imageUsesRegistry(image, policy.Registry) {
			result.ExternalImages = append(result.ExternalImages, image)
		}
	}
	if policy.Enforce && len(result.ExternalImages) > 0 {
		return result, fmt.Errorf(
			"offline image registry check failed: %d rendered image(s) do not use %s: %s",
			len(result.ExternalImages),
			policy.Registry,
			strings.Join(result.ExternalImages, ", "),
		)
	}
	return result, nil
}

func ExtractManifestImages(manifest string) []string {
	images := []string{}
	for _, doc := range splitManifestDocuments(manifest) {
		if isCommentOnlyManifestDocument(doc) {
			continue
		}
		var u unstructured.Unstructured
		if err := yaml.Unmarshal([]byte(doc), &u.Object); err != nil || u.GetKind() == "" {
			continue
		}
		images = append(images, extractImagesFromObject(u.Object)...)
	}
	return uniqueSortedStrings(images)
}

func cloneValuesMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		if nested, ok := value.(map[string]interface{}); ok {
			out[key] = cloneValuesMap(nested)
			continue
		}
		out[key] = value
	}
	return out
}

func extractImagesFromObject(obj map[string]interface{}) []string {
	images := []string{}
	visitPodSpec := func(podSpec map[string]interface{}) {
		for _, field := range []string{"initContainers", "containers", "ephemeralContainers"} {
			items, _, _ := unstructured.NestedSlice(podSpec, field)
			for _, item := range items {
				container, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				image, _, _ := unstructured.NestedString(container, "image")
				image = strings.TrimSpace(image)
				if image != "" {
					images = append(images, image)
				}
			}
		}
	}

	kind, _, _ := unstructured.NestedString(obj, "kind")
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "pod":
		if podSpec, ok, _ := unstructured.NestedMap(obj, "spec"); ok {
			visitPodSpec(podSpec)
		}
	case "deployment", "statefulset", "daemonset", "replicaset", "replicationcontroller":
		if podSpec, ok, _ := unstructured.NestedMap(obj, "spec", "template", "spec"); ok {
			visitPodSpec(podSpec)
		}
	case "job":
		if podSpec, ok, _ := unstructured.NestedMap(obj, "spec", "template", "spec"); ok {
			visitPodSpec(podSpec)
		}
	case "cronjob":
		if podSpec, ok, _ := unstructured.NestedMap(obj, "spec", "jobTemplate", "spec", "template", "spec"); ok {
			visitPodSpec(podSpec)
		}
	}
	return images
}

func imageUsesRegistry(image, registry string) bool {
	imageRegistry := imageRegistryHost(image)
	if imageRegistry == "" {
		imageRegistry = "docker.io"
	}
	return strings.EqualFold(imageRegistry, registry)
}

func imageRegistryHost(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	first := strings.Split(image, "/")[0]
	if first == "" {
		return ""
	}
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" || net.ParseIP(first) != nil {
		return first
	}
	return "docker.io"
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
