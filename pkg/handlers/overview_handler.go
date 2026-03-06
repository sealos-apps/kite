package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OverviewData struct {
	TotalNodes      int                   `json:"totalNodes"`
	ReadyNodes      int                   `json:"readyNodes"`
	TotalPods       int                   `json:"totalPods"`
	RunningPods     int                   `json:"runningPods"`
	TotalNamespaces int                   `json:"totalNamespaces"`
	TotalIngresses  int                   `json:"totalIngresses"`
	TotalPVCs       int                   `json:"totalPVCs"`
	TotalServices   int                   `json:"totalServices"`
	PromEnabled     bool                  `json:"prometheusEnabled"`
	Resource        common.ResourceMetric `json:"resource"`
}

func GetOverview(c *gin.Context) {
	ctx := c.Request.Context()

	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if len(user.Roles) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
	}

	// Get nodes
	nodes := &v1.NodeList{}
	if err := cs.K8sClient.List(ctx, nodes, &client.ListOptions{}); err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			klog.Warningf("overview: skip nodes for cluster %s due to permission: %v", cs.Name, err)
			nodes = &v1.NodeList{}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	readyNodes := 0
	var cpuAllocatable, memAllocatable resource.Quantity
	var cpuRequested, memRequested resource.Quantity
	var cpuLimited, memLimited resource.Quantity
	cpuBasis := common.ResourceBasisClusterAllocatable
	memoryBasis := common.ResourceBasisClusterAllocatable
	for _, node := range nodes.Items {
		cpuAllocatable.Add(*node.Status.Allocatable.Cpu())
		memAllocatable.Add(*node.Status.Allocatable.Memory())
		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
				readyNodes++
				break
			}
		}
	}

	// Get pods
	pods := &v1.PodList{}
	podListOptions := &client.ListOptions{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		podListOptions.Namespace = cs.Namespace
	}
	if err := cs.K8sClient.List(ctx, pods, podListOptions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	runningPods := 0
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			cpuRequested.Add(*container.Resources.Requests.Cpu())
			memRequested.Add(*container.Resources.Requests.Memory())

			if container.Resources.Limits != nil {
				if cpuLimit := container.Resources.Limits.Cpu(); cpuLimit != nil {
					cpuLimited.Add(*cpuLimit)
				}
				if memLimit := container.Resources.Limits.Memory(); memLimit != nil {
					memLimited.Add(*memLimit)
				}
			}
		}
		if pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded {
			runningPods++
		}
	}

	if cs.NamespaceScoped && cs.Namespace != "" {
		var quotaList v1.ResourceQuotaList
		if err := cs.K8sClient.List(ctx, &quotaList, client.InNamespace(cs.Namespace)); err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				klog.Warningf("overview: skip resourcequotas for namespace %s due to permission: %v", cs.Namespace, err)
			} else {
				klog.Warningf("overview: failed to list resourcequotas for namespace %s: %v", cs.Namespace, err)
			}
		} else {
			cpuQuotaMilli, memoryQuotaBytes, hasCPUQuota, hasMemoryQuota, err := extractNamespaceQuotaHard(quotaList.Items)
			if err != nil {
				klog.Warningf("overview: failed to parse resourcequotas for namespace %s: %v", cs.Namespace, err)
			} else {
				if hasCPUQuota {
					cpuAllocatable = *resource.NewMilliQuantity(cpuQuotaMilli, resource.DecimalSI)
					cpuBasis = common.ResourceBasisNamespaceQuota
				} else {
					cpuBasis = common.ResourceBasisNamespaceNoQuota
				}
				if hasMemoryQuota {
					memAllocatable = *resource.NewQuantity(memoryQuotaBytes, resource.BinarySI)
					memoryBasis = common.ResourceBasisNamespaceQuota
				} else {
					memoryBasis = common.ResourceBasisNamespaceNoQuota
				}
			}
		}
	}

	// Get namespaces
	namespaces := &v1.NamespaceList{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		namespaces.Items = append(namespaces.Items, v1.Namespace{})
	} else if err := cs.K8sClient.List(ctx, namespaces, &client.ListOptions{}); err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			klog.Warningf("overview: skip namespaces for cluster %s due to permission: %v", cs.Name, err)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Get services
	services := &v1.ServiceList{}
	serviceListOptions := &client.ListOptions{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		serviceListOptions.Namespace = cs.Namespace
	}
	if err := cs.K8sClient.List(ctx, services, serviceListOptions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get ingresses
	ingresses := &networkingv1.IngressList{}
	ingressListOptions := &client.ListOptions{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		ingressListOptions.Namespace = cs.Namespace
	}
	if err := cs.K8sClient.List(ctx, ingresses, ingressListOptions); err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			klog.Warningf("overview: skip ingresses for cluster %s due to permission: %v", cs.Name, err)
			ingresses = &networkingv1.IngressList{}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Get persistentvolumeclaims
	pvcs := &v1.PersistentVolumeClaimList{}
	pvcListOptions := &client.ListOptions{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		pvcListOptions.Namespace = cs.Namespace
	}
	if err := cs.K8sClient.List(ctx, pvcs, pvcListOptions); err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			klog.Warningf("overview: skip persistentvolumeclaims for cluster %s due to permission: %v", cs.Name, err)
			pvcs = &v1.PersistentVolumeClaimList{}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	overview := OverviewData{
		TotalNodes:      len(nodes.Items),
		ReadyNodes:      readyNodes,
		TotalPods:       len(pods.Items),
		RunningPods:     runningPods,
		TotalNamespaces: len(namespaces.Items),
		TotalIngresses:  len(ingresses.Items),
		TotalPVCs:       len(pvcs.Items),
		TotalServices:   len(services.Items),
		PromEnabled:     cs.PromClient != nil,
		Resource: common.ResourceMetric{
			CPU: common.Resource{
				Allocatable: cpuAllocatable.MilliValue(),
				Requested:   cpuRequested.MilliValue(),
				Limited:     cpuLimited.MilliValue(),
				Basis:       cpuBasis,
			},
			Mem: common.Resource{
				Allocatable: memAllocatable.MilliValue(),
				Requested:   memRequested.MilliValue(),
				Limited:     memLimited.MilliValue(),
				Basis:       memoryBasis,
			},
		},
	}

	c.JSON(http.StatusOK, overview)
}

// var (
// 	initialized bool
// )

func InitCheck(c *gin.Context) {
	// if initialized {
	// 	c.JSON(http.StatusOK, gin.H{"initialized": true})
	// 	return
	// }
	step := 0
	uc, _ := model.CountUsers()
	if uc == 0 && !common.AnonymousUserEnabled {
		c.SetCookie("auth_token", "", -1, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"initialized": false, "step": step})
		return
	}
	if uc > 0 || common.AnonymousUserEnabled {
		step++
	}
	cc, _ := model.CountClusters()
	if cc > 0 {
		step++
	}
	initialized := step == 2
	c.JSON(http.StatusOK, gin.H{"initialized": initialized, "step": step})
}
