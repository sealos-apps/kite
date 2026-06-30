package helmutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	release "helm.sh/helm/v4/pkg/release/v1"
)

func TestNormalizeChartSourceInfersOCIFromChartURL(t *testing.T) {
	require.Equal(t, ChartSourceOCI, NormalizeChartSource("", "oci://registry.internal/kite-helm/nginx:1.0.0"))
	require.Equal(t, ChartSourceRepository, NormalizeChartSource(ChartSourceRepository, "oci://registry.internal/kite-helm/nginx:1.0.0"))
	require.Empty(t, NormalizeChartSource("", "https://charts.example.com/nginx-1.0.0.tgz"))
}

func TestReleaseChartSourceReadsAnnotatedChartSource(t *testing.T) {
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: "nginx", Version: "1.0.0"}}
	AnnotateChartSource(ch, ChartProvenance{
		Source:         ChartSourceOCI,
		RepositoryName: "cluster62",
		ChartName:      "nginx",
		Version:        "1.0.0",
		URL:            "oci://registry.internal/kite-helm/nginx:1.0.0",
	})

	rel := &release.Release{Chart: ch}
	require.Equal(t, ChartSourceOCI, ReleaseChartSource(rel))
	require.Equal(t, ChartSourceOCI, ch.Metadata.Annotations[chartSourceAnnotation])
	require.Equal(t, "cluster62", ReleaseChartProvenance(rel).RepositoryName)
	require.Equal(t, "oci://registry.internal/kite-helm/nginx:1.0.0", ReleaseChartProvenance(rel).URL)
}

func TestPrepareReleaseValuesInjectsGlobalImageRegistryForOCI(t *testing.T) {
	originalEnabled := common.HelmOfflineImagesEnabled
	originalRegistry := common.HelmOfflineImagesRegistry
	originalEnforce := common.HelmOfflineImagesEnforce
	t.Cleanup(func() {
		common.HelmOfflineImagesEnabled = originalEnabled
		common.HelmOfflineImagesRegistry = originalRegistry
		common.HelmOfflineImagesEnforce = originalEnforce
	})

	common.HelmOfflineImagesEnabled = true
	common.HelmOfflineImagesRegistry = "hub.192.168.0.62.nip.io"
	common.HelmOfflineImagesEnforce = true

	values, policy, injected := PrepareReleaseValues(map[string]interface{}{
		"replicaCount": 1,
	}, ChartSourceOCI)
	require.True(t, policy.Enabled)
	require.True(t, injected)
	global := values["global"].(map[string]interface{})
	require.Equal(t, "hub.192.168.0.62.nip.io", global["imageRegistry"])
	require.Equal(t, true, global["security"].(map[string]interface{})["allowInsecureImages"])
}

func TestPrepareReleaseValuesKeepsUserImageRegistry(t *testing.T) {
	originalEnabled := common.HelmOfflineImagesEnabled
	originalRegistry := common.HelmOfflineImagesRegistry
	t.Cleanup(func() {
		common.HelmOfflineImagesEnabled = originalEnabled
		common.HelmOfflineImagesRegistry = originalRegistry
	})

	common.HelmOfflineImagesEnabled = true
	common.HelmOfflineImagesRegistry = "hub.local"

	values, _, injected := PrepareReleaseValues(map[string]interface{}{
		"global": map[string]interface{}{
			"imageRegistry": "custom.local",
			"security": map[string]interface{}{
				"allowInsecureImages": false,
			},
		},
	}, ChartSourceOCI)
	require.False(t, injected)
	global := values["global"].(map[string]interface{})
	require.Equal(t, "custom.local", global["imageRegistry"])
	require.Equal(t, false, global["security"].(map[string]interface{})["allowInsecureImages"])
}

func TestCheckReleaseImagesRejectsExternalImages(t *testing.T) {
	rel := &release.Release{
		Namespace: "default",
		Manifest: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  template:
    spec:
      initContainers:
      - name: wait
        image: docker.io/bitnami/os-shell:12
      containers:
      - name: nginx
        image: hub.192.168.0.62.nip.io/bitnami/nginx:1.29.1
`,
	}

	result, err := CheckReleaseImages(rel, OfflineImagePolicy{
		Enabled:  true,
		Registry: "hub.192.168.0.62.nip.io",
		Enforce:  true,
	}, true)
	require.Error(t, err)
	require.Equal(t, []string{
		"docker.io/bitnami/os-shell:12",
	}, result.ExternalImages)
	require.Equal(t, []string{
		"docker.io/bitnami/os-shell:12",
		"hub.192.168.0.62.nip.io/bitnami/nginx:1.29.1",
	}, result.AllImages)
}

func TestCheckReleaseImagesAllowsOfflineRegistryImages(t *testing.T) {
	rel := &release.Release{
		Namespace: "default",
		Manifest: `
apiVersion: batch/v1
kind: CronJob
metadata:
  name: cleanup
spec:
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: cleanup
            image: hub.local/bitnami/kubectl:1.31
`,
	}

	result, err := CheckReleaseImages(rel, OfflineImagePolicy{
		Enabled:  true,
		Registry: "hub.local",
		Enforce:  true,
	}, false)
	require.NoError(t, err)
	require.Empty(t, result.ExternalImages)
	require.Equal(t, []string{"hub.local/bitnami/kubectl:1.31"}, result.AllImages)
}
