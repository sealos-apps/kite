package resources

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestTableToSummaryListMapsPodRowsAndFiltersNamespaces(t *testing.T) {
	table := &v1.Table{
		ColumnDefinitions: []v1.TableColumnDefinition{
			{Name: "Namespace"},
			{Name: "Name"},
			{Name: "Ready"},
			{Name: "Status"},
			{Name: "Restarts"},
			{Name: "Age"},
			{Name: "IP"},
			{Name: "Node"},
		},
		Rows: []v1.TableRow{
			{Cells: []any{"default", "api-0", "1/2", "Running", "3", "2h", "10.0.0.1", "node-a"}},
			{Cells: []any{"blocked", "api-1", "1/1", "Running", "0", "1h", "10.0.0.2", "node-b"}},
		},
	}
	meta := common.MustLookupResource(string(common.Pods))
	user := model.User{Roles: []common.Role{
		{Name: "one-ns", Clusters: []string{"cluster-a"}, Namespaces: []string{"default"}},
	}}
	now := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)

	got := tableToSummaryList(table, meta, common.AllNamespaces, user, "cluster-a", now)

	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
	item := got.Items[0]
	metadata := asMap(t, item["metadata"])
	if metadata["name"] != "api-0" || metadata["namespace"] != "default" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if metadata["creationTimestamp"] != "2026-07-03T06:00:00Z" {
		t.Fatalf("creationTimestamp = %v", metadata["creationTimestamp"])
	}
	spec := asMap(t, item["spec"])
	if spec["nodeName"] != "node-a" {
		t.Fatalf("nodeName = %v", spec["nodeName"])
	}
	if containers := spec["containers"].([]map[string]any); len(containers) != 2 {
		t.Fatalf("containers = %d, want 2", len(containers))
	}
	status := asMap(t, item["status"])
	if status["podIP"] != "10.0.0.1" || status["reason"] != "Running" {
		t.Fatalf("unexpected status: %#v", status)
	}
	containerStatuses := status["containerStatuses"].([]map[string]any)
	if len(containerStatuses) != 2 {
		t.Fatalf("containerStatuses = %d, want 2", len(containerStatuses))
	}
	if containerStatuses[0]["restartCount"] != 3 {
		t.Fatalf("restartCount = %v, want 3", containerStatuses[0]["restartCount"])
	}
}

func TestTableToSummaryListMapsServiceRows(t *testing.T) {
	table := &v1.Table{
		ColumnDefinitions: []v1.TableColumnDefinition{
			{Name: "Name"},
			{Name: "Type"},
			{Name: "Cluster-IP"},
			{Name: "External-IP"},
			{Name: "Port(s)"},
			{Name: "Age"},
		},
		Rows: []v1.TableRow{
			{Cells: []any{"web", "NodePort", "10.0.0.10", "1.2.3.4", "80:30080/TCP, 443/TCP", "30m"}},
		},
	}
	meta := common.MustLookupResource(string(common.Services))
	user := model.User{Roles: []common.Role{
		{Name: "default", Clusters: []string{"cluster-a"}, Namespaces: []string{"default"}},
	}}
	now := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)

	got := tableToSummaryList(table, meta, "default", user, "cluster-a", now)

	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
	item := got.Items[0]
	metadata := asMap(t, item["metadata"])
	if metadata["namespace"] != "default" {
		t.Fatalf("namespace = %v", metadata["namespace"])
	}
	spec := asMap(t, item["spec"])
	if spec["type"] != "NodePort" || spec["clusterIP"] != "10.0.0.10" {
		t.Fatalf("unexpected spec: %#v", spec)
	}
	externalIPs := spec["externalIPs"].([]string)
	if len(externalIPs) != 1 || externalIPs[0] != "1.2.3.4" {
		t.Fatalf("externalIPs = %#v", externalIPs)
	}
	ports := spec["ports"].([]map[string]any)
	if len(ports) != 2 {
		t.Fatalf("ports = %#v", ports)
	}
	if ports[0]["port"] != 80 || ports[0]["nodePort"] != 30080 || ports[1]["port"] != 443 {
		t.Fatalf("unexpected ports: %#v", ports)
	}
}

func TestTableToSummaryListMapsNodeRows(t *testing.T) {
	table := &v1.Table{
		ColumnDefinitions: []v1.TableColumnDefinition{
			{Name: "Name"},
			{Name: "Status"},
			{Name: "Roles"},
			{Name: "Age"},
			{Name: "Version"},
			{Name: "Internal-IP"},
		},
		Rows: []v1.TableRow{
			{Cells: []any{"node-a", "Ready,SchedulingDisabled", "control-plane,worker", "1d", "v1.33.0", "10.0.0.10"}},
		},
	}
	meta := common.MustLookupResource(string(common.Nodes))
	user := model.User{Roles: []common.Role{
		{Name: "cluster", Clusters: []string{"cluster-a"}, Namespaces: []string{common.AllNamespaces}},
	}}
	now := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)

	got := tableToSummaryList(table, meta, "", user, "cluster-a", now)

	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
	item := got.Items[0]
	metadata := asMap(t, item["metadata"])
	labels := metadata["labels"].(map[string]string)
	if _, ok := labels["node-role.kubernetes.io/control-plane"]; !ok {
		t.Fatalf("expected control-plane role label, got %#v", labels)
	}
	spec := asMap(t, item["spec"])
	if spec["unschedulable"] != true {
		t.Fatalf("expected unschedulable node spec, got %#v", spec)
	}
	status := asMap(t, item["status"])
	nodeInfo := asMap(t, status["nodeInfo"])
	if nodeInfo["kubeletVersion"] != "v1.33.0" {
		t.Fatalf("unexpected nodeInfo: %#v", nodeInfo)
	}
	addresses := status["addresses"].([]map[string]any)
	if len(addresses) != 1 || addresses[0]["address"] != "10.0.0.10" {
		t.Fatalf("unexpected addresses: %#v", addresses)
	}
}

func TestTableSummaryURLUsesResourcePathAndListOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/services/default?limit=999&continue=next&labelSelector=app%3Dweb&fieldSelector=metadata.name%3Dweb", nil)
	ctx.Params = gin.Params{{Key: "namespace", Value: "default"}}
	meta := common.MustLookupResource(string(common.Services))

	got, err := tableSummaryURL(&rest.Config{Host: "https://cluster.example/root"}, meta, ctx)
	if err != nil {
		t.Fatalf("tableSummaryURL returned error: %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if parsed.Path != "/root/api/v1/namespaces/default/services" {
		t.Fatalf("path = %q", parsed.Path)
	}
	query := parsed.Query()
	if query.Get("limit") != "500" {
		t.Fatalf("limit = %q, want clamp to 500", query.Get("limit"))
	}
	if query.Get("continue") != "next" || query.Get("labelSelector") != "app=web" || query.Get("fieldSelector") != "metadata.name=web" {
		t.Fatalf("unexpected query: %s", parsed.RawQuery)
	}
}

func TestShouldFallbackFromSummaryErrorOnlyForUnsupportedTableResponses(t *testing.T) {
	if !shouldFallbackFromSummaryError(errSummaryUnsupported) {
		t.Fatalf("expected unsupported summary to fall back")
	}
	if !shouldFallbackFromSummaryError(&tableSummaryStatusError{statusCode: http.StatusNotAcceptable, status: "406 Not Acceptable"}) {
		t.Fatalf("expected 406 table response to fall back")
	}
	if shouldFallbackFromSummaryError(&tableSummaryStatusError{statusCode: http.StatusInternalServerError, status: "500 Internal Server Error"}) {
		t.Fatalf("must not fall back on server errors")
	}
	if shouldFallbackFromSummaryError(contextDeadlineExceededError{}) {
		t.Fatalf("must not fall back on transport timeout errors")
	}
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want map[string]any", value)
	}
	return result
}

type contextDeadlineExceededError struct{}

func (contextDeadlineExceededError) Error() string { return "context deadline exceeded" }
