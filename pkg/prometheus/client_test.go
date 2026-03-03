package prometheus

import "testing"

func TestBuildResourceUsageQueriesWithNamespaceScopedCluster(t *testing.T) {
	cpuQuery, memoryQuery, networkInQuery, networkOutQuery := buildResourceUsageQueries(
		"node-a",
		"instance",
		ResourceUsageOptions{Namespace: "ns-a"},
	)

	if got, want := cpuQuery, `sum(rate(container_cpu_usage_seconds_total{container!="POD",container!="",namespace="ns-a",instance="node-a"}[1m])) / sum(kube_node_status_allocatable{resource="cpu",node="node-a"}) * 100`; got != want {
		t.Fatalf("cpu query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := memoryQuery, `sum(container_memory_usage_bytes{container!="POD",container!="",namespace="ns-a",instance="node-a"}) / sum(kube_node_status_allocatable{resource="memory",node="node-a"}) * 100`; got != want {
		t.Fatalf("memory query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkInQuery, `sum(rate(container_network_receive_bytes_total{namespace="ns-a",instance="node-a"}[1m]))`; got != want {
		t.Fatalf("network in query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkOutQuery, `sum(rate(container_network_transmit_bytes_total{namespace="ns-a",instance="node-a"}[1m]))`; got != want {
		t.Fatalf("network out query mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestBuildResourceUsageQueriesWithoutNamespaceOrInstance(t *testing.T) {
	cpuQuery, memoryQuery, networkInQuery, networkOutQuery := buildResourceUsageQueries(
		"",
		"instance",
		ResourceUsageOptions{},
	)

	if got, want := cpuQuery, `sum(rate(container_cpu_usage_seconds_total{container!="POD",container!=""}[1m])) / sum(kube_node_status_allocatable{resource="cpu"}) * 100`; got != want {
		t.Fatalf("cpu query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := memoryQuery, `sum(container_memory_usage_bytes{container!="POD",container!=""}) / sum(kube_node_status_allocatable{resource="memory"}) * 100`; got != want {
		t.Fatalf("memory query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkInQuery, `sum(rate(container_network_receive_bytes_total{}[1m]))`; got != want {
		t.Fatalf("network in query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkOutQuery, `sum(rate(container_network_transmit_bytes_total{}[1m]))`; got != want {
		t.Fatalf("network out query mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestBuildResourceUsageQueriesWithNamespaceQuotaDenominators(t *testing.T) {
	cpuQuery, memoryQuery, networkInQuery, networkOutQuery := buildResourceUsageQueries(
		"",
		"instance",
		ResourceUsageOptions{
			Namespace:          "ns-a",
			CPUCapacityCores:   4,
			MemoryCapacityByte: 8589934592, // 8Gi
		},
	)

	if got, want := cpuQuery, `sum(rate(container_cpu_usage_seconds_total{container!="POD",container!="",namespace="ns-a"}[1m])) / 4 * 100`; got != want {
		t.Fatalf("cpu query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := memoryQuery, `sum(container_memory_usage_bytes{container!="POD",container!="",namespace="ns-a"}) / 8589934592 * 100`; got != want {
		t.Fatalf("memory query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkInQuery, `sum(rate(container_network_receive_bytes_total{namespace="ns-a"}[1m]))`; got != want {
		t.Fatalf("network in query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkOutQuery, `sum(rate(container_network_transmit_bytes_total{namespace="ns-a"}[1m]))`; got != want {
		t.Fatalf("network out query mismatch\nwant: %s\ngot:  %s", want, got)
	}
}
