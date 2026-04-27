package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var builtinSidebarCRDNames = []string{
	"apps.app.sealos.io",
	"devboxes.devbox.sealos.io",
	"rabbitmqclusters.rabbitmq.com",
	"elasticsearches.elasticsearch.k8s.elastic.co",
	"clusters.apps.kubeblocks.io",
}

type sidebarCRDVersion struct {
	Name                     string                                           `json:"name"`
	Served                   bool                                             `json:"served"`
	Storage                  bool                                             `json:"storage"`
	AdditionalPrinterColumns []apiextensionsv1.CustomResourceColumnDefinition `json:"additionalPrinterColumns,omitempty"`
}

type sidebarCRDInfo struct {
	Name     string              `json:"name"`
	Kind     string              `json:"kind"`
	Group    string              `json:"group"`
	Scope    string              `json:"scope"`
	Versions []sidebarCRDVersion `json:"versions"`
}

func ListBuiltinSidebarCRDs(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	ctx := c.Request.Context()
	items := make([]sidebarCRDInfo, 0, len(builtinSidebarCRDNames))

	for _, crdName := range builtinSidebarCRDNames {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := cs.K8sClient.Get(ctx, types.NamespacedName{Name: crdName}, &crd); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		versions := make([]sidebarCRDVersion, 0, len(crd.Spec.Versions))
		for _, version := range crd.Spec.Versions {
			versions = append(versions, sidebarCRDVersion{
				Name:                     version.Name,
				Served:                   version.Served,
				Storage:                  version.Storage,
				AdditionalPrinterColumns: version.AdditionalPrinterColumns,
			})
		}

		items = append(items, sidebarCRDInfo{
			Name:     crd.Name,
			Kind:     crd.Spec.Names.Kind,
			Group:    crd.Spec.Group,
			Scope:    string(crd.Spec.Scope),
			Versions: versions,
		})
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}
