package prometheus

import (
	"fmt"
	"testing"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

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
	if got, want := networkInQuery, `sum(rate(node_network_receive_bytes_total{device!="lo"}[1m]))`; got != want {
		t.Fatalf("network in query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkOutQuery, `sum(rate(node_network_transmit_bytes_total{device!="lo"}[1m]))`; got != want {
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

func TestBuildResourceUsageQueriesDisallowClusterFallback(t *testing.T) {
	cpuQuery, memoryQuery, networkInQuery, networkOutQuery := buildResourceUsageQueries(
		"",
		"instance",
		ResourceUsageOptions{
			Namespace:                       "ns-a",
			DisallowClusterCapacityFallback: true,
		},
	)

	if got, want := cpuQuery, ``; got != want {
		t.Fatalf("cpu query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := memoryQuery, ``; got != want {
		t.Fatalf("memory query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkInQuery, `sum(rate(container_network_receive_bytes_total{namespace="ns-a"}[1m]))`; got != want {
		t.Fatalf("network in query mismatch\nwant: %s\ngot:  %s", want, got)
	}
	if got, want := networkOutQuery, `sum(rate(container_network_transmit_bytes_total{namespace="ns-a"}[1m]))`; got != want {
		t.Fatalf("network out query mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestIsForbiddenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "wrapped forbidden client error",
			err: fmt.Errorf("error querying CPU usage: %w",
				&v1.Error{Type: v1.ErrClient, Msg: "client error: 403"}),
			want: true,
		},
		{
			name: "client error but not forbidden",
			err:  &v1.Error{Type: v1.ErrClient, Msg: "client error: 401"},
			want: false,
		},
		{
			name: "non client error",
			err:  &v1.Error{Type: v1.ErrServer, Msg: "server error: 500"},
			want: false,
		},
		{
			name: "generic error",
			err:  fmt.Errorf("any other error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsForbiddenError(tt.err)
			if got != tt.want {
				t.Fatalf("IsForbiddenError() = %v, want %v", got, tt.want)
			}
		})
	}
}
