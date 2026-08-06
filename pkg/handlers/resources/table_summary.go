package resources

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

const tableSummaryAccept = "application/json;as=Table;g=meta.k8s.io;v=v1;includeObject=None"

var agePartPattern = regexp.MustCompile(`(\d+)([smhdy])`)
var errSummaryUnsupported = errors.New("summary list is not supported")

type tableSummaryStatusError struct {
	statusCode int
	status     string
	body       string
}

func (e *tableSummaryStatusError) Error() string {
	if e.body == "" {
		return "table summary request failed: " + e.status
	}
	return "table summary request failed: " + e.status + ": " + e.body
}

type summaryListResponse struct {
	APIVersion string           `json:"apiVersion,omitempty"`
	Kind       string           `json:"kind,omitempty"`
	Items      []map[string]any `json:"items"`
	Metadata   v1.ListMeta      `json:"metadata,omitempty"`
}

type tableRowReader struct {
	columns map[string]int
	row     v1.TableRow
}

func wantsSummaryList(c *gin.Context) bool {
	return c.Query("summary") == "true" && c.Query("reduce") == "true"
}

func (h *GenericResourceHandler[T, V]) summaryList(c *gin.Context) (*summaryListResponse, error) {
	if !wantsSummaryList(c) {
		return nil, fmt.Errorf("summary list is not requested")
	}

	meta := common.LookupResource(h.name)
	if meta == nil {
		return nil, fmt.Errorf("%w for %s", errSummaryUnsupported, h.name)
	}

	table, err := h.fetchTableSummary(c, meta)
	if err != nil {
		return nil, err
	}

	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	namespace := c.Param("namespace")
	return tableToSummaryList(table, meta, namespace, user, cs.Name, time.Now()), nil
}

func (h *GenericResourceHandler[T, V]) fetchTableSummary(c *gin.Context, meta *common.ResourceMeta) (*v1.Table, error) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	reqURL, err := tableSummaryURL(cs.K8sClient.Configuration, meta, c)
	if err != nil {
		return nil, err
	}

	httpClient := cs.K8sClient.HTTPClient
	if httpClient == nil {
		var err error
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

	var table v1.Table
	if err := json.NewDecoder(resp.Body).Decode(&table); err != nil {
		return nil, err
	}
	return &table, nil
}

func shouldFallbackFromSummaryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errSummaryUnsupported) {
		return true
	}

	var statusErr *tableSummaryStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.statusCode {
		case http.StatusNotFound, http.StatusNotAcceptable, http.StatusUnsupportedMediaType:
			return true
		}
	}
	return false
}

func writeSummaryListError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if strings.Contains(err.Error(), "invalid limit parameter") {
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func tableSummaryURL(config *rest.Config, meta *common.ResourceMeta, c *gin.Context) (string, error) {
	return tableSummaryURLForGVR(
		config,
		schema.GroupVersionResource{
			Group:    meta.Group,
			Version:  meta.Version,
			Resource: string(meta.Plural),
		},
		!meta.ClusterScoped,
		c,
	)
}

func tableSummaryURLForGVR(config *rest.Config, gvr schema.GroupVersionResource, namespaced bool, c *gin.Context) (string, error) {
	if config == nil || strings.TrimSpace(config.Host) == "" {
		return "", fmt.Errorf("missing Kubernetes REST config host")
	}
	parsed, err := url.Parse(config.Host)
	if err != nil {
		return "", err
	}

	namespace := c.Param("namespace")
	segments := []string{}
	if gvr.Group == "" {
		segments = append(segments, "api", gvr.Version)
	} else {
		segments = append(segments, "apis", gvr.Group, gvr.Version)
	}
	if namespaced && namespace != "" && namespace != common.AllNamespaces {
		segments = append(segments, "namespaces", namespace)
	}
	segments = append(segments, gvr.Resource)

	escaped := make([]string, 0, len(segments))
	for _, segment := range segments {
		escaped = append(escaped, url.PathEscape(segment))
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.Join(escaped, "/")

	query := parsed.Query()
	if limit, enabled, err := normalizeListLimit(c.Query("limit")); err != nil {
		return "", err
	} else if enabled {
		query.Set("limit", strconv.FormatInt(limit, 10))
	}
	for _, key := range []string{"continue", "labelSelector", "fieldSelector"} {
		if value := c.Query(key); value != "" {
			query.Set(key, value)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func tableToSummaryList(table *v1.Table, meta *common.ResourceMeta, namespace string, user model.User, clusterName string, now time.Time) *summaryListResponse {
	if table == nil {
		table = &v1.Table{}
	}

	result := &summaryListResponse{
		APIVersion: resourceAPIVersion(meta),
		Kind:       meta.Kind + "List",
		Metadata:   table.ListMeta,
		Items:      make([]map[string]any, 0, len(table.Rows)),
	}

	columns := tableColumnIndex(table.ColumnDefinitions)
	for _, row := range table.Rows {
		reader := tableRowReader{columns: columns, row: row}
		itemNamespace := reader.cell("Namespace")
		if itemNamespace == "" && !meta.ClusterScoped && namespace != common.AllNamespaces {
			itemNamespace = namespace
		}
		name := reader.cell("Name")
		if name == "" {
			name = reader.firstNonNamespaceCell()
		}
		if name == "" {
			continue
		}
		if meta.Plural == common.Namespaces {
			if !rbac.CanAccessNamespace(user, clusterName, name) {
				continue
			}
		}
		if namespace == common.AllNamespaces && itemNamespace != "" && !rbac.CanAccessNamespace(user, clusterName, itemNamespace) {
			continue
		}

		item := baseSummaryItem(meta, name, itemNamespace, reader.cell("Age"), now)
		applyTableCellsToSummary(item, meta.Plural, reader, now)
		result.Items = append(result.Items, item)
	}

	return result
}

func tableColumnIndex(definitions []v1.TableColumnDefinition) map[string]int {
	columns := make(map[string]int, len(definitions))
	for i, definition := range definitions {
		columns[normalizeTableColumnName(definition.Name)] = i
	}
	return columns
}

func normalizeTableColumnName(name string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func (r tableRowReader) cell(name string) string {
	idx, ok := r.columns[normalizeTableColumnName(name)]
	if !ok || idx < 0 || idx >= len(r.row.Cells) {
		return ""
	}
	return cellString(r.row.Cells[idx])
}

func (r tableRowReader) firstNonNamespaceCell() string {
	for i, cell := range r.row.Cells {
		if r.columns[normalizeTableColumnName("Namespace")] == i {
			continue
		}
		value := cellString(cell)
		if value != "" {
			return value
		}
	}
	return ""
}

func cellString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func baseSummaryItem(meta *common.ResourceMeta, name, namespace, age string, now time.Time) map[string]any {
	metadata := map[string]any{
		"name": name,
	}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	if timestamp := timestampFromAge(age, now); timestamp != "" {
		metadata["creationTimestamp"] = timestamp
	}
	return map[string]any{
		"apiVersion": resourceAPIVersion(meta),
		"kind":       meta.Kind,
		"metadata":   metadata,
	}
}

func resourceAPIVersion(meta *common.ResourceMeta) string {
	if meta.Group == "" {
		return meta.Version
	}
	return meta.Group + "/" + meta.Version
}

func applyTableCellsToSummary(item map[string]any, resource common.ResourceType, reader tableRowReader, now time.Time) {
	switch resource {
	case common.Pods:
		applyPodTableSummary(item, reader)
	case common.Nodes:
		applyNodeTableSummary(item, reader)
	case common.Services:
		applyServiceTableSummary(item, reader)
	case common.Namespaces:
		applyNamespaceTableSummary(item, reader)
	case common.Deployments:
		applyDeploymentTableSummary(item, reader)
	case common.ReplicaSets:
		applyReplicaSetTableSummary(item, reader)
	case common.StatefulSets:
		applyStatefulSetTableSummary(item, reader)
	case common.DaemonSets:
		applyDaemonSetTableSummary(item, reader)
	case common.Jobs:
		applyJobTableSummary(item, reader, now)
	case common.CronJobs:
		applyCronJobTableSummary(item, reader, now)
	case common.PersistentVolumes:
		applyPVTableSummary(item, reader)
	case common.PersistentVolumeClaims:
		applyPVCTableSummary(item, reader)
	case common.Secrets:
		item["type"] = emptyAsDefault(reader.cell("Type"), string(corev1.SecretTypeOpaque))
	case common.ServiceAccounts:
		secretCount := parseLeadingInt(reader.cell("Secrets"))
		item["secrets"] = make([]map[string]any, max(0, secretCount))
	}
}

func applyPodTableSummary(item map[string]any, reader tableRowReader) {
	ready, total := parseRatio(reader.cell("Ready"))
	status := emptyAsDefault(reader.cell("Status"), "Unknown")
	restarts := parseLeadingInt(reader.cell("Restarts"))

	item["spec"] = map[string]any{
		"nodeName":   reader.cell("Node"),
		"containers": summaryContainers(total),
	}
	item["status"] = map[string]any{
		"phase":                 status,
		"reason":                status,
		"podIP":                 dashAsEmpty(reader.cell("IP")),
		"containerStatuses":     summaryContainerStatuses(total, ready, restarts, status),
		"initContainerStatuses": []map[string]any{},
	}
}

func applyNodeTableSummary(item map[string]any, reader tableRowReader) {
	statusText := reader.cell("Status")
	ready := strings.Contains(statusText, "Ready") && !strings.Contains(statusText, "NotReady")
	conditionStatus := string(corev1.ConditionFalse)
	if ready {
		conditionStatus = string(corev1.ConditionTrue)
	}

	metadata := item["metadata"].(map[string]any)
	if labels := nodeRoleLabels(reader.cell("Roles")); len(labels) > 0 {
		metadata["labels"] = labels
	}

	spec := map[string]any{}
	if strings.Contains(statusText, "SchedulingDisabled") {
		spec["unschedulable"] = true
	}
	item["spec"] = spec
	item["status"] = map[string]any{
		"conditions": []map[string]any{
			{
				"type":   string(corev1.NodeReady),
				"status": conditionStatus,
			},
		},
		"nodeInfo": map[string]any{
			"kubeletVersion": reader.cell("Version"),
			"kernelVersion":  reader.cell("Kernel Version"),
			"osImage":        reader.cell("OS Image"),
		},
		"addresses": nodeAddresses(reader),
	}
}

func applyServiceTableSummary(item map[string]any, reader tableRowReader) {
	serviceType := emptyAsDefault(reader.cell("Type"), string(corev1.ServiceTypeClusterIP))
	clusterIP := dashAsEmpty(reader.cell("Cluster-IP"))
	externalIP := dashAsEmpty(reader.cell("External-IP"))
	spec := map[string]any{
		"type":      serviceType,
		"clusterIP": clusterIP,
		"ports":     parseServicePorts(reader.cell("Port(s)")),
	}
	status := map[string]any{}

	switch corev1.ServiceType(serviceType) {
	case corev1.ServiceTypeLoadBalancer:
		if externalIP != "" {
			status["loadBalancer"] = map[string]any{
				"ingress": []map[string]any{ipOrHostname(externalIP)},
			}
		}
	case corev1.ServiceTypeExternalName:
		spec["externalName"] = externalIP
	default:
		if externalIP != "" {
			spec["externalIPs"] = []string{externalIP}
		}
	}

	item["spec"] = spec
	item["status"] = status
}

func nodeRoleLabels(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "<none>" {
		return nil
	}
	labels := map[string]string{}
	for _, role := range strings.Split(raw, ",") {
		role = strings.TrimSpace(role)
		if role == "" || role == "<none>" {
			continue
		}
		labels["node-role.kubernetes.io/"+role] = ""
	}
	return labels
}

func nodeAddresses(reader tableRowReader) []map[string]any {
	addresses := []map[string]any{}
	if internalIP := dashAsEmpty(reader.cell("Internal-IP")); internalIP != "" {
		addresses = append(addresses, map[string]any{"type": string(corev1.NodeInternalIP), "address": internalIP})
	}
	if externalIP := dashAsEmpty(reader.cell("External-IP")); externalIP != "" {
		addresses = append(addresses, map[string]any{"type": string(corev1.NodeExternalIP), "address": externalIP})
	}
	return addresses
}

func applyNamespaceTableSummary(item map[string]any, reader tableRowReader) {
	item["status"] = map[string]any{
		"phase": emptyAsDefault(reader.cell("Status"), "Unknown"),
	}
}

func applyDeploymentTableSummary(item map[string]any, reader tableRowReader) {
	ready, desired := parseRatio(reader.cell("Ready"))
	available := parseLeadingInt(reader.cell("Available"))
	if desired == 0 && reader.cell("Ready") == "" {
		desired = parseLeadingInt(reader.cell("Desired"))
	}
	item["spec"] = map[string]any{
		"replicas": desired,
	}
	item["status"] = map[string]any{
		"replicas":          desired,
		"readyReplicas":     ready,
		"availableReplicas": available,
		"updatedReplicas":   parseLeadingInt(reader.cell("Up-to-date")),
	}
}

func applyReplicaSetTableSummary(item map[string]any, reader tableRowReader) {
	desired := parseLeadingInt(reader.cell("Desired"))
	item["spec"] = map[string]any{
		"replicas": desired,
	}
	item["status"] = map[string]any{
		"replicas":      parseLeadingInt(reader.cell("Current")),
		"readyReplicas": parseLeadingInt(reader.cell("Ready")),
	}
}

func applyStatefulSetTableSummary(item map[string]any, reader tableRowReader) {
	ready, desired := parseRatio(reader.cell("Ready"))
	item["spec"] = map[string]any{
		"replicas": desired,
	}
	item["status"] = map[string]any{
		"replicas":      desired,
		"readyReplicas": ready,
	}
}

func applyDaemonSetTableSummary(item map[string]any, reader tableRowReader) {
	desired := parseLeadingInt(reader.cell("Desired"))
	item["status"] = map[string]any{
		"desiredNumberScheduled": desired,
		"currentNumberScheduled": parseLeadingInt(reader.cell("Current")),
		"numberReady":            parseLeadingInt(reader.cell("Ready")),
		"updatedNumberScheduled": parseLeadingInt(reader.cell("Up-to-date")),
		"numberAvailable":        parseLeadingInt(reader.cell("Available")),
	}
}

func applyJobTableSummary(item map[string]any, reader tableRowReader, now time.Time) {
	succeeded, completions := parseRatio(reader.cell("Completions"))
	statusText := reader.cell("Status")
	if completions == 0 {
		completions = 1
	}
	status := map[string]any{
		"succeeded": succeeded,
	}
	if strings.EqualFold(statusText, "Complete") || strings.EqualFold(statusText, "Completed") {
		status["conditions"] = []map[string]any{{"type": "Complete", "status": "True"}}
		if timestamp := timestampFromAge(reader.cell("Duration"), now); timestamp != "" {
			status["completionTime"] = now.UTC().Format(time.RFC3339)
			status["startTime"] = timestamp
		}
	} else if strings.EqualFold(statusText, "Failed") {
		status["conditions"] = []map[string]any{{"type": "Failed", "status": "True"}}
	} else {
		status["conditions"] = []map[string]any{}
	}
	item["spec"] = map[string]any{
		"completions": completions,
	}
	item["status"] = status
}

func applyCronJobTableSummary(item map[string]any, reader tableRowReader, now time.Time) {
	spec := map[string]any{
		"schedule": reader.cell("Schedule"),
		"suspend":  strings.EqualFold(reader.cell("Suspend"), "True"),
	}
	status := map[string]any{
		"active": make([]map[string]any, max(0, parseLeadingInt(reader.cell("Active")))),
	}
	if timestamp := timestampFromAge(reader.cell("Last Schedule"), now); timestamp != "" {
		status["lastScheduleTime"] = timestamp
	}
	item["spec"] = spec
	item["status"] = status
}

func applyPVTableSummary(item map[string]any, reader tableRowReader) {
	item["spec"] = map[string]any{
		"storageClassName":              dashAsEmpty(reader.cell("StorageClass")),
		"accessModes":                   splitList(reader.cell("Access Modes")),
		"persistentVolumeReclaimPolicy": reader.cell("Reclaim Policy"),
		"claimRef":                      claimRefFromCell(reader.cell("Claim")),
	}
	item["status"] = map[string]any{
		"phase": reader.cell("Status"),
	}
}

func applyPVCTableSummary(item map[string]any, reader tableRowReader) {
	item["spec"] = map[string]any{
		"storageClassName": dashAsEmpty(reader.cell("StorageClass")),
		"accessModes":      splitList(reader.cell("Access Modes")),
		"volumeName":       dashAsEmpty(reader.cell("Volume")),
	}
	item["status"] = map[string]any{
		"phase": reader.cell("Status"),
	}
}

func summaryContainers(total int) []map[string]any {
	containers := make([]map[string]any, max(0, total))
	for i := range containers {
		containers[i] = map[string]any{"name": fmt.Sprintf("container-%d", i+1)}
	}
	return containers
}

func summaryContainerStatuses(total, ready, restarts int, status string) []map[string]any {
	statuses := make([]map[string]any, max(0, total))
	for i := range statuses {
		state := map[string]any{"waiting": map[string]any{"reason": status}}
		if i < ready && strings.EqualFold(status, string(corev1.PodRunning)) {
			state = map[string]any{"running": map[string]any{"startedAt": time.Now().UTC().Format(time.RFC3339)}}
		}
		statuses[i] = map[string]any{
			"name":         fmt.Sprintf("container-%d", i+1),
			"ready":        i < ready,
			"restartCount": 0,
			"state":        state,
		}
	}
	if len(statuses) > 0 && restarts > 0 {
		statuses[0]["restartCount"] = restarts
	}
	return statuses
}

func parseServicePorts(raw string) []map[string]any {
	raw = dashAsEmpty(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	ports := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part)
		if text == "" {
			continue
		}
		protocol := "TCP"
		if slash := strings.LastIndex(text, "/"); slash >= 0 {
			protocol = strings.TrimSpace(text[slash+1:])
			text = strings.TrimSpace(text[:slash])
		}
		portPart := text
		nodePort := 0
		if colon := strings.LastIndex(text, ":"); colon >= 0 {
			portPart = strings.TrimSpace(text[:colon])
			nodePort = parseLeadingInt(text[colon+1:])
		}
		port := parseLeadingInt(portPart)
		if port == 0 {
			continue
		}
		item := map[string]any{
			"port":     port,
			"protocol": emptyAsDefault(protocol, "TCP"),
		}
		if nodePort > 0 {
			item["nodePort"] = nodePort
		}
		ports = append(ports, item)
	}
	return ports
}

func ipOrHostname(value string) map[string]any {
	if strings.ContainsAny(value, ":") || regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`).MatchString(value) {
		return map[string]any{"ip": value}
	}
	return map[string]any{"hostname": value}
}

func claimRefFromCell(raw string) map[string]any {
	raw = dashAsEmpty(raw)
	if raw == "" {
		return nil
	}
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 {
		return map[string]any{"name": raw}
	}
	return map[string]any{"namespace": parts[0], "name": parts[1]}
}

func parseRatio(raw string) (int, int) {
	parts := strings.Split(strings.TrimSpace(raw), "/")
	if len(parts) != 2 {
		value := parseLeadingInt(raw)
		return value, value
	}
	return parseLeadingInt(parts[0]), parseLeadingInt(parts[1])
}

func parseLeadingInt(raw string) int {
	raw = strings.TrimSpace(raw)
	match := regexp.MustCompile(`\d+`).FindString(raw)
	if match == "" {
		return 0
	}
	value, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}
	return value
}

func timestampFromAge(age string, now time.Time) string {
	duration := durationFromAge(age)
	if duration <= 0 {
		return ""
	}
	return now.Add(-duration).UTC().Format(time.RFC3339)
}

func durationFromAge(age string) time.Duration {
	age = strings.TrimSpace(age)
	if age == "" || age == "<unknown>" || age == "<none>" {
		return 0
	}

	var total time.Duration
	for _, match := range agePartPattern.FindAllStringSubmatch(age, -1) {
		value, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		switch match[2] {
		case "s":
			total += time.Duration(value) * time.Second
		case "m":
			total += time.Duration(value) * time.Minute
		case "h":
			total += time.Duration(value) * time.Hour
		case "d":
			total += time.Duration(value) * 24 * time.Hour
		case "y":
			total += time.Duration(value) * 365 * 24 * time.Hour
		}
	}
	return total
}

func splitList(raw string) []string {
	raw = dashAsEmpty(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			result = append(result, field)
		}
	}
	return result
}

func dashAsEmpty(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "", "-", "<none>", "<unknown>":
		return ""
	default:
		return value
	}
}

func emptyAsDefault(value, fallback string) string {
	if value = dashAsEmpty(value); value != "" {
		return value
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
