package resources

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func (h *CRHandler) summaryList(c *gin.Context, crd *apiextensionsv1.CustomResourceDefinition, gvr schema.GroupVersionResource) (*summaryListResponse, error) {
	if !wantsSummaryList(c) {
		return nil, fmt.Errorf("summary list is not requested")
	}
	table, err := h.fetchTableSummary(c, crd, gvr)
	if err != nil {
		return nil, err
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	return customResourceTableToSummaryList(table, crd, gvr, c.Param("namespace"), user, cs.Name, time.Now()), nil
}

func (h *CRHandler) fetchTableSummary(c *gin.Context, crd *apiextensionsv1.CustomResourceDefinition, gvr schema.GroupVersionResource) (*metav1.Table, error) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	reqURL, err := tableSummaryURLForGVR(
		cs.K8sClient.Configuration,
		gvr,
		crd.Spec.Scope == apiextensionsv1.NamespaceScoped,
		c,
	)
	if err != nil {
		return nil, err
	}

	httpClient := cs.K8sClient.HTTPClient
	if httpClient == nil {
		httpClient, err = rest.HTTPClientFor(cs.K8sClient.Configuration)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", tableSummaryAccept)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &tableSummaryStatusError{
			statusCode: resp.StatusCode,
			status:     resp.Status,
			body:       strings.TrimSpace(string(body)),
		}
	}

	var table metav1.Table
	if err := json.NewDecoder(resp.Body).Decode(&table); err != nil {
		return nil, err
	}
	return &table, nil
}

func customResourceTableToSummaryList(
	table *metav1.Table,
	crd *apiextensionsv1.CustomResourceDefinition,
	gvr schema.GroupVersionResource,
	namespace string,
	user model.User,
	clusterName string,
	now time.Time,
) *summaryListResponse {
	if table == nil {
		table = &metav1.Table{}
	}
	result := &summaryListResponse{
		APIVersion: gvr.GroupVersion().String(),
		Kind:       crd.Spec.Names.ListKind,
		Metadata:   table.ListMeta,
		Items:      make([]map[string]any, 0, len(table.Rows)),
	}
	columns := tableColumnIndex(table.ColumnDefinitions)
	printerColumns := customResourcePrinterColumns(crd, gvr.Version)
	printerIndexes := matchCustomPrinterColumnIndexes(table.ColumnDefinitions, printerColumns)

	for _, row := range table.Rows {
		reader := tableRowReader{columns: columns, row: row}
		itemNamespace := reader.cell("Namespace")
		if itemNamespace == "" && crd.Spec.Scope == apiextensionsv1.NamespaceScoped && namespace != common.AllNamespaces {
			itemNamespace = namespace
		}
		if namespace == common.AllNamespaces && itemNamespace != "" && !rbac.CanAccessNamespace(user, clusterName, itemNamespace) {
			continue
		}
		name := reader.cell("Name")
		if name == "" {
			name = reader.firstNonNamespaceCell()
		}
		if name == "" {
			continue
		}

		metadata := map[string]any{"name": name}
		if itemNamespace != "" {
			metadata["namespace"] = itemNamespace
		}
		if timestamp := timestampFromAge(reader.cell("Age"), now); timestamp != "" {
			metadata["creationTimestamp"] = timestamp
		}
		printerValues := make(map[string]any, len(printerColumns))
		for _, definition := range printerColumns {
			value := ""
			if index, ok := printerIndexes[definition.JSONPath]; ok && index < len(row.Cells) {
				value = cellString(row.Cells[index])
			}
			printerValues[definition.JSONPath] = value
		}
		result.Items = append(result.Items, map[string]any{
			"apiVersion":          gvr.GroupVersion().String(),
			"kind":                crd.Spec.Names.Kind,
			"metadata":            metadata,
			"_kitePrinterColumns": printerValues,
		})
	}
	return result
}

func customResourcePrinterColumns(crd *apiextensionsv1.CustomResourceDefinition, version string) []apiextensionsv1.CustomResourceColumnDefinition {
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Name == version {
			return crd.Spec.Versions[i].AdditionalPrinterColumns
		}
	}
	return nil
}

func matchCustomPrinterColumnIndexes(
	tableColumns []metav1.TableColumnDefinition,
	printerColumns []apiextensionsv1.CustomResourceColumnDefinition,
) map[string]int {
	result := make(map[string]int, len(printerColumns))
	used := make(map[int]struct{}, len(printerColumns))
	searchStart := 0
	for _, printer := range printerColumns {
		matched := -1
		for pass := 0; pass < 2 && matched < 0; pass++ {
			start := searchStart
			if pass == 1 {
				start = 0
			}
			for index := start; index < len(tableColumns); index++ {
				if _, exists := used[index]; exists {
					continue
				}
				candidate := tableColumns[index]
				if normalizeTableColumnName(candidate.Name) != normalizeTableColumnName(printer.Name) {
					continue
				}
				if printer.Type != "" && candidate.Type != "" && !strings.EqualFold(candidate.Type, printer.Type) {
					continue
				}
				matched = index
				break
			}
		}
		if matched >= 0 {
			result[printer.JSONPath] = matched
			used[matched] = struct{}{}
			searchStart = matched + 1
		}
	}
	return result
}
