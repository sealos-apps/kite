package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmutil"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	syaml "sigs.k8s.io/yaml"
)

type ResourceApplyHandler struct {
}

func NewResourceApplyHandler() *ResourceApplyHandler {
	return &ResourceApplyHandler{}
}

type ApplyResourceRequest struct {
	YAML string `json:"yaml" binding:"required"`
}

type applyResourceMetadata struct {
	Resource      string
	ClusterScoped bool
}

// ApplyResource applies a YAML resource to the cluster
func (h *ResourceApplyHandler) ApplyResource(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)

	var req ApplyResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Decode YAML into unstructured object
	decodeUniversal := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}

	_, _, err := decodeUniversal.Decode([]byte(req.YAML), nil, obj)
	if err != nil {
		klog.Errorf("Failed to decode YAML: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid YAML format: " + err.Error()})
		return
	}

	ctx := c.Request.Context()

	resourceMeta, err := resolveApplyResourceMetadata(cs, obj)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	namespace, err := normalizeApplyResourceNamespace(cs, resourceMeta, obj)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	resource := resourceMeta.Resource

	existingObj := &unstructured.Unstructured{}
	existingObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	existingObj.SetName(obj.GetName())
	existingObj.SetNamespace(namespace)

	lookupErr := cs.K8sClient.Get(ctx, client.ObjectKey{
		Name:      obj.GetName(),
		Namespace: namespace,
	}, existingObj)
	verb := string(common.VerbUpdate)
	if apierrors.IsNotFound(lookupErr) {
		verb = string(common.VerbCreate)
	}
	if !rbac.CanAccess(user, resource, verb, cs.Name, namespace) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": rbac.NoAccess(user.Key(), verb, resource, namespace, cs.Name)})
		return
	}

	var applyErr error
	defer func() {
		previousYAML := []byte{}
		if existingObj.GetResourceVersion() != "" {
			existingObj.SetManagedFields(nil)
			previousYAML, _ = syaml.Marshal(existingObj)
		}
		errMessage := ""
		if applyErr != nil {
			errMessage = applyErr.Error()
		}
		model.DB.Create(&model.ResourceHistory{
			ClusterName:   cs.Name,
			ResourceType:  resource,
			ResourceName:  obj.GetName(),
			Namespace:     namespace,
			OperationType: "apply",
			ResourceYAML:  req.YAML,
			PreviousYAML:  string(previousYAML),
			OperatorID:    user.ID,
			Success:       applyErr == nil,
			ErrorMessage:  errMessage,
		})
	}()

	switch {
	case apierrors.IsNotFound(lookupErr):
		applyErr = cs.K8sClient.Create(ctx, obj)
		if applyErr != nil {
			klog.Errorf("Failed to create resource: %v", applyErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create resource: " + applyErr.Error()})
			return
		}
	case lookupErr == nil:
		obj.SetResourceVersion(existingObj.GetResourceVersion())
		applyErr = cs.K8sClient.Update(ctx, obj)
		if applyErr != nil {
			klog.Errorf("Failed to update resource: %v", applyErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resource: " + applyErr.Error()})
			return
		}
	default:
		applyErr = lookupErr
		klog.Errorf("Failed to get resource: %v", applyErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get resource: " + applyErr.Error()})
		return
	}

	klog.Infof("Successfully applied resource: %s/%s", obj.GetKind(), obj.GetName())
	c.JSON(http.StatusOK, gin.H{
		"message":   "Resource applied successfully",
		"kind":      obj.GetKind(),
		"name":      obj.GetName(),
		"namespace": obj.GetNamespace(),
	})
}

func resolveApplyResourceMetadata(cs *cluster.ClientSet, obj *unstructured.Unstructured) (applyResourceMetadata, error) {
	if obj.GetKind() == "" || obj.GetName() == "" {
		return applyResourceMetadata{}, fmt.Errorf("yaml must include kind and metadata.name")
	}
	if resource := common.LookupResource(obj.GetKind()); resource != nil {
		return applyResourceMetadata{
			Resource:      string(resource.Plural),
			ClusterScoped: resource.ClusterScoped,
		}, nil
	}

	gvk := obj.GroupVersionKind()
	if gvk.Empty() {
		return applyResourceMetadata{}, fmt.Errorf("yaml for %s/%s must include apiVersion and kind", obj.GetKind(), obj.GetName())
	}
	if cs == nil || cs.K8sClient == nil || cs.K8sClient.ClientSet == nil {
		return applyResourceMetadata{}, fmt.Errorf("cluster discovery client is not available")
	}

	if resource, ok := discoverApplyResourceMetadata(cs, gvk); ok {
		return resource, nil
	}
	return applyResourceMetadata{}, fmt.Errorf("resource %s %s/%s is not recognized by cluster discovery", gvk.String(), obj.GetNamespace(), obj.GetName())
}

func discoverApplyResourceMetadata(cs *cluster.ClientSet, gvk schema.GroupVersionKind) (applyResourceMetadata, bool) {
	discoveryClient := cs.K8sClient.ClientSet.Discovery()
	if gv := gvk.GroupVersion(); !gv.Empty() {
		resourceList, err := discoveryClient.ServerResourcesForGroupVersion(gv.String())
		if err == nil {
			if resource, ok := findApplyResourceMetadata(gvk, resourceList.APIResources); ok {
				return resource, true
			}
		} else {
			klog.V(2).Infof("failed to discover resources for %s: %v", gv.String(), err)
		}
	}

	resourceLists, err := discoveryClient.ServerPreferredResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		klog.V(2).Infof("failed to discover preferred resources: %v", err)
		return applyResourceMetadata{}, false
	}
	for _, resourceList := range resourceLists {
		if resourceList == nil {
			continue
		}
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil || gv.Group != gvk.Group || gv.Version != gvk.Version {
			continue
		}
		if resource, ok := findApplyResourceMetadata(gvk, resourceList.APIResources); ok {
			return resource, true
		}
	}

	mapping, err := restMappingForApplyResource(cs, gvk)
	if err == nil && mapping != nil {
		return applyResourceMetadata{
			Resource:      mapping.Resource.Resource,
			ClusterScoped: mapping.Scope.Name() == meta.RESTScopeNameRoot,
		}, true
	}
	return applyResourceMetadata{}, false
}

func findApplyResourceMetadata(gvk schema.GroupVersionKind, resources []metav1.APIResource) (applyResourceMetadata, bool) {
	for _, apiResource := range resources {
		if strings.Contains(apiResource.Name, "/") || !strings.EqualFold(apiResource.Kind, gvk.Kind) {
			continue
		}
		return applyResourceMetadata{
			Resource:      apiResource.Name,
			ClusterScoped: !apiResource.Namespaced,
		}, true
	}
	return applyResourceMetadata{}, false
}

func restMappingForApplyResource(cs *cluster.ClientSet, gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	if cs == nil || cs.K8sClient == nil || cs.K8sClient.Configuration == nil {
		return nil, fmt.Errorf("rest config is not available")
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, "")
	if err != nil {
		return nil, err
	}
	mapper, err := cfg.RESTClientGetter.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	return mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
}

func normalizeApplyResourceNamespace(cs *cluster.ClientSet, resource applyResourceMetadata, obj *unstructured.Unstructured) (string, error) {
	namespace := strings.TrimSpace(obj.GetNamespace())
	if resource.ClusterScoped {
		obj.SetNamespace("")
		if isApplyNamespaceScopeLocked(cs) {
			return "", fmt.Errorf("resource %s is cluster-scoped and is not available in namespace-scoped workspaces", resource.Resource)
		}
		return "", nil
	}

	if isApplyNamespaceScopeLocked(cs) {
		scopedNamespace := strings.TrimSpace(cs.Namespace)
		if namespace == "" || namespace == common.AllNamespaces {
			obj.SetNamespace(scopedNamespace)
			return scopedNamespace, nil
		}
		if namespace != scopedNamespace {
			return "", fmt.Errorf("namespace %s is outside the current workspace scope %s", namespace, scopedNamespace)
		}
	}
	return namespace, nil
}

func isApplyNamespaceScopeLocked(cs *cluster.ClientSet) bool {
	return cs != nil && cs.NamespaceScoped && strings.TrimSpace(cs.Namespace) != "" &&
		!common.IsNamespaceScopeExempt(cs.Namespace)
}
