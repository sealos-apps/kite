package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCRHandlerListHistoryFindsPluralHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	t.Cleanup(func() {
		model.DB = previousDB
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ResourceHistory{}))
	model.DB = db

	user := model.User{Username: "tester", Provider: "password"}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, db.Create(&model.ResourceHistory{
		ClusterName:   "test-cluster",
		ResourceType:  "teams",
		ResourceName:  "team-a",
		Namespace:     "",
		OperationType: "apply",
		ResourceYAML:  "kind: Team\nmetadata:\n  name: team-a\n",
		Success:       true,
		OperatorID:    user.ID,
		CreatedAt:     time.Now(),
	}).Error)

	scheme := runtime.NewScheme()
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "teams.maestro.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "maestro.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "Team",
				Plural: "teams",
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("cluster", &cluster.ClientSet{
			Name: "test-cluster",
			K8sClient: &kube.K8sClient{
				Client: fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(crd).
					Build(),
			},
		})
		c.Set("user", user)
	})
	RegisterRoutes(router.Group("/api/v1"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams.maestro.io/_all/team-a/history?page=1&pageSize=10", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []model.ResourceHistory `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	require.Equal(t, "teams", response.Data[0].ResourceType)
	require.Equal(t, "team-a", response.Data[0].ResourceName)
}
