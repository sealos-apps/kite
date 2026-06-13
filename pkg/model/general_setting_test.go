package model

import (
	"testing"

	"github.com/zxh326/kite/pkg/common"
)

func TestDefaultGeneralNodeTerminalImageValue(t *testing.T) {
	original := common.NodeTerminalImage
	t.Cleanup(func() {
		common.NodeTerminalImage = original
	})

	common.NodeTerminalImage = "  custom/node-terminal:1.0  "
	if got := DefaultGeneralNodeTerminalImageValue(); got != "custom/node-terminal:1.0" {
		t.Fatalf("DefaultGeneralNodeTerminalImageValue() = %q, want %q", got, "custom/node-terminal:1.0")
	}

	common.NodeTerminalImage = "   "
	if got := DefaultGeneralNodeTerminalImageValue(); got != DefaultGeneralNodeTerminalImage {
		t.Fatalf("DefaultGeneralNodeTerminalImageValue() = %q, want %q", got, DefaultGeneralNodeTerminalImage)
	}
}

func TestDefaultGeneralSettingEnablesAIAgent(t *testing.T) {
	setting := defaultGeneralSetting()
	if !setting.AIAgentEnabled {
		t.Fatalf("defaultGeneralSetting().AIAgentEnabled = false, want true")
	}
}

func TestNormalizeGeneralAIProvider(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"anthropic", " Anthropic ", GeneralAIProviderAnthropic},
		{"openai", "OPENAI", GeneralAIProviderOpenAI},
		{"unknown", "something-else", GeneralAIProviderOpenAI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeGeneralAIProvider(tt.input); got != tt.expected {
				t.Fatalf("NormalizeGeneralAIProvider() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsGeneralAIProviderSupported(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"openai", "openai", true},
		{"anthropic", " Anthropic ", true},
		{"unknown", "gemini", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGeneralAIProviderSupported(tt.input); got != tt.want {
				t.Fatalf("IsGeneralAIProviderSupported() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultGeneralAIModelByProvider(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"anthropic", GeneralAIProviderAnthropic, DefaultGeneralAnthropicModel},
		{"openai", GeneralAIProviderOpenAI, DefaultGeneralAIModel},
		{"unknown", "anything else", DefaultGeneralAIModel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultGeneralAIModelByProvider(tt.input); got != tt.expected {
				t.Fatalf("DefaultGeneralAIModelByProvider() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestApplyRuntimeGeneralSetting(t *testing.T) {
	originalAnalytics := common.EnableAnalytics
	originalVersionCheck := common.EnableVersionCheck
	originalDisableVersionCheck := common.DisableVersionCheck
	t.Cleanup(func() {
		common.EnableAnalytics = originalAnalytics
		common.EnableVersionCheck = originalVersionCheck
		common.DisableVersionCheck = originalDisableVersionCheck
	})

	applyRuntimeGeneralSetting(&GeneralSetting{
		EnableAnalytics:    true,
		EnableVersionCheck: false,
	})

	if !common.EnableAnalytics {
		t.Fatalf("EnableAnalytics = %v, want true", common.EnableAnalytics)
	}
	if common.EnableVersionCheck {
		t.Fatalf("EnableVersionCheck = %v, want false", common.EnableVersionCheck)
	}
	if !common.DisableVersionCheck {
		t.Fatalf("DisableVersionCheck = %v, want true", common.DisableVersionCheck)
	}

	applyRuntimeGeneralSetting(nil)
	if !common.EnableAnalytics {
		t.Fatalf("nil setting changed EnableAnalytics")
	}
	if common.EnableVersionCheck {
		t.Fatalf("nil setting changed EnableVersionCheck")
	}
	if !common.DisableVersionCheck {
		t.Fatalf("nil setting changed DisableVersionCheck")
	}
}
