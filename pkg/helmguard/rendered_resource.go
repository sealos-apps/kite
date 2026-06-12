package helmguard

import (
	"context"
	"fmt"
	"strings"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmutil"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	release "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

func AuthorizeCreateNamespace(user model.User, cs *cluster.ClientSet, createNamespace bool) error {
	if !createNamespace {
		return nil
	}
	if cs.NamespaceScoped && cs.Namespace != "" && !common.IsNamespaceScopeExempt(cs.Namespace) {
		return fmt.Errorf("createNamespace is not allowed on namespace-scoped clusters")
	}
	if !rbac.UserHasRole(user, model.DefaultAdminRole.Name) && !rbac.CanAccess(user, string(common.Namespaces), string(common.VerbCreate), cs.Name, "") {
		return fmt.Errorf("createNamespace requires namespace create permission")
	}
	return nil
}

func AuthorizeRelease(ctx context.Context, user model.User, cs *cluster.ClientSet, rel *release.Release, verb string) error {
	resources := helmutil.ReleaseManifestResources(rel)
	for _, resource := range resources {
		if err := AuthorizeResource(ctx, user, cs, resource, rel.Namespace, verb); err != nil {
			return err
		}
	}
	return nil
}

func AuthorizeReleaseChange(ctx context.Context, user model.User, cs *cluster.ClientSet, current, next *release.Release) error {
	currentResources := indexedReleaseResources(current)
	nextResources := indexedReleaseResources(next)

	for key, resource := range nextResources {
		verb := string(common.VerbCreate)
		if _, exists := currentResources[key]; exists {
			verb = string(common.VerbUpdate)
		}
		if err := AuthorizeResource(ctx, user, cs, resource, next.Namespace, verb); err != nil {
			return err
		}
	}

	for key, resource := range currentResources {
		if _, exists := nextResources[key]; exists {
			continue
		}
		if err := AuthorizeResource(ctx, user, cs, resource, current.Namespace, string(common.VerbDelete)); err != nil {
			return err
		}
	}

	return nil
}

func AuthorizeReleaseDelete(ctx context.Context, user model.User, cs *cluster.ClientSet, rel *release.Release) error {
	return AuthorizeRelease(ctx, user, cs, rel, string(common.VerbDelete))
}

func AuthorizeResource(ctx context.Context, user model.User, cs *cluster.ClientSet, resource helmutil.HelmReleaseResource, releaseNamespace, verb string) error {
	namespace := strings.TrimSpace(resource.Namespace)
	resourceName, clusterScoped, err := renderedResourceRBACName(ctx, cs, resource)
	if err != nil {
		return err
	}
	if !clusterScoped {
		if namespace == "" {
			namespace = releaseNamespace
		}
		if namespace == "" || namespace == common.AllNamespaces {
			return fmt.Errorf("rendered %s/%s is missing namespace", resource.Kind, resource.Name)
		}
		if namespace != releaseNamespace {
			return fmt.Errorf("rendered %s/%s targets namespace %s, expected %s", resource.Kind, resource.Name, namespace, releaseNamespace)
		}
		if cs.NamespaceScoped && cs.Namespace != "" && !common.IsNamespaceScopeExempt(cs.Namespace) && namespace != cs.Namespace {
			return fmt.Errorf("rendered %s/%s targets namespace %s outside cluster scope %s", resource.Kind, resource.Name, namespace, cs.Namespace)
		}
	} else {
		namespace = ""
		if cs.NamespaceScoped && cs.Namespace != "" && !common.IsNamespaceScopeExempt(cs.Namespace) {
			return fmt.Errorf("rendered %s/%s is cluster-scoped and is not allowed on namespace-scoped clusters", resource.Kind, resource.Name)
		}
		if !rbac.UserHasRole(user, model.DefaultAdminRole.Name) {
			return fmt.Errorf("rendered %s/%s is cluster-scoped and requires admin role", resource.Kind, resource.Name)
		}
	}
	if !rbac.CanAccess(user, resourceName, verb, cs.Name, namespace) {
		return fmt.Errorf("%s; rendered %s/%s from Helm chart", rbac.NoAccess(user.Key(), verb, resourceName, namespace, cs.Name), resource.Kind, resource.Name)
	}
	return nil
}

func indexedReleaseResources(rel *release.Release) map[string]helmutil.HelmReleaseResource {
	out := map[string]helmutil.HelmReleaseResource{}
	if rel == nil {
		return out
	}
	for _, resource := range helmutil.ReleaseManifestResources(rel) {
		out[renderedResourceKey(resource, rel.Namespace)] = resource
	}
	return out
}

func renderedResourceKey(resource helmutil.HelmReleaseResource, releaseNamespace string) string {
	namespace := strings.TrimSpace(resource.Namespace)
	if namespace == "" && !helmutil.IsManifestClusterScopedKind(resource.Kind) {
		namespace = strings.TrimSpace(releaseNamespace)
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(resource.APIVersion)),
		strings.ToLower(strings.TrimSpace(resource.Kind)),
		strings.ToLower(namespace),
		strings.ToLower(strings.TrimSpace(resource.Name)),
	}, "\x00")
}

func renderedResourceRBACName(ctx context.Context, cs *cluster.ClientSet, resource helmutil.HelmReleaseResource) (string, bool, error) {
	if metaResource := common.LookupResource(resource.Kind); metaResource != nil {
		return string(metaResource.Plural), metaResource.ClusterScoped, nil
	}
	if helmutil.IsManifestClusterScopedKind(resource.Kind) {
		resourceName := strings.ToLower(resource.Kind) + "s"
		if resource.Kind == "CustomResourceDefinition" {
			resourceName = string(common.CRDs)
		}
		return resourceName, true, nil
	}
	gvk := schema.FromAPIVersionAndKind(resource.APIVersion, resource.Kind)
	if gvk.Empty() {
		return "", false, fmt.Errorf("rendered resource %s/%s is missing apiVersion or kind", resource.Kind, resource.Name)
	}
	if cs.K8sClient == nil || cs.K8sClient.ClientSet == nil {
		return "", false, fmt.Errorf("cluster discovery client is not available")
	}
	discoveryClient := cs.K8sClient.ClientSet.Discovery()
	lists, err := discoveryClient.ServerPreferredResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return "", false, err
	}
	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil || gv.Group != gvk.Group || gv.Version != gvk.Version {
			continue
		}
		for _, apiResource := range list.APIResources {
			if strings.Contains(apiResource.Name, "/") || !strings.EqualFold(apiResource.Kind, gvk.Kind) {
				continue
			}
			return apiResource.Name, !apiResource.Namespaced, nil
		}
	}
	mapping, err := restMappingForRenderedResource(cs, gvk)
	if err == nil && mapping != nil {
		return mapping.Resource.Resource, mapping.Scope.Name() == meta.RESTScopeNameRoot, nil
	}
	return "", false, fmt.Errorf("rendered resource %s %s/%s is not recognized by cluster discovery", gvk.String(), resource.Namespace, resource.Name)
}

func restMappingForRenderedResource(cs *cluster.ClientSet, gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, "")
	if err != nil {
		return nil, err
	}
	mapper, err := cfg.RESTClientGetter.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	if mapper == nil {
		return nil, fmt.Errorf("rest mapper not available")
	}
	return mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
}
