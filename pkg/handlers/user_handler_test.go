package handlers

import "testing"

func TestHasNewCustomGroup(t *testing.T) {
	tests := []struct {
		name              string
		currentPreference string
		nextPreference    string
		want              bool
		wantErr           bool
	}{
		{
			name:              "no custom group in new preference",
			currentPreference: `{"groups":[{"id":"sidebar-groups-workloads"}]}`,
			nextPreference:    `{"groups":[{"id":"sidebar-groups-workloads"},{"id":"sidebar-groups-config"}]}`,
			want:              false,
		},
		{
			name:              "existing custom group only",
			currentPreference: `{"groups":[{"id":"custom-crds","isCustom":true}]}`,
			nextPreference:    `{"groups":[{"id":"custom-crds","isCustom":true},{"id":"sidebar-groups-config"}]}`,
			want:              false,
		},
		{
			name:              "new custom group is added",
			currentPreference: `{"groups":[{"id":"custom-crds","isCustom":true}]}`,
			nextPreference:    `{"groups":[{"id":"custom-crds","isCustom":true},{"id":"custom-network","isCustom":true}]}`,
			want:              true,
		},
		{
			name:              "custom prefix is treated as custom group",
			currentPreference: `{"groups":[{"id":"custom-crds","isCustom":true}]}`,
			nextPreference:    `{"groups":[{"id":"custom-crds","isCustom":true},{"id":"custom-ops"}]}`,
			want:              true,
		},
		{
			name:              "invalid next preference json",
			currentPreference: `{"groups":[{"id":"custom-crds","isCustom":true}]}`,
			nextPreference:    `{"groups":[`,
			wantErr:           true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := hasNewCustomGroup(tc.currentPreference, tc.nextPreference)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("hasNewCustomGroup() = %v, want %v", got, tc.want)
			}
		})
	}
}
