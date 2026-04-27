package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListBuiltinSidebarCRDsReturnsExistingBuiltins(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "devboxes.devbox.sealos.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "devbox.sealos.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "devboxes",
				Kind:   "Devbox",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:     "Status",
							Type:     "string",
							JSONPath: ".status.phase",
						},
					},
				},
			},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sidebar/builtin-crds", nil)
	c.Set("cluster", &cluster.ClientSet{
		K8sClient: &kube.K8sClient{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(crd).
				Build(),
		},
	})

	ListBuiltinSidebarCRDs(c)

	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Items []sidebarCRDInfo `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	assert.Equal(t, "devboxes.devbox.sealos.io", response.Items[0].Name)
	assert.Equal(t, "Devbox", response.Items[0].Kind)
	assert.Equal(t, string(apiextensionsv1.NamespaceScoped), response.Items[0].Scope)
	require.Len(t, response.Items[0].Versions, 1)
	assert.Equal(t, "Status", response.Items[0].Versions[0].AdditionalPrinterColumns[0].Name)
}
