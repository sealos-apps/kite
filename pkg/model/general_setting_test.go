package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"gorm.io/gorm"
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
	if !setting.AIAgentConfigured {
		t.Fatalf("defaultGeneralSetting().AIAgentConfigured = false, want true")
	}
}

func TestGetGeneralSettingUpgradesUnconfiguredAIAgentDefault(t *testing.T) {
	useTestGeneralSettingDB(t)

	legacySetting := defaultGeneralSetting()
	if err := DB.Create(&legacySetting).Error; err != nil {
		t.Fatalf("create legacy general setting: %v", err)
	}
	if err := DB.Model(&GeneralSetting{}).Where("id = ?", legacySetting.ID).Updates(map[string]interface{}{
		"ai_agent_enabled":    false,
		"ai_agent_configured": false,
	}).Error; err != nil {
		t.Fatalf("force legacy AI agent state: %v", err)
	}

	setting, err := GetGeneralSetting()
	if err != nil {
		t.Fatalf("GetGeneralSetting() error = %v", err)
	}
	if !setting.AIAgentEnabled {
		t.Fatalf("GetGeneralSetting().AIAgentEnabled = false, want true")
	}
	if !setting.AIAgentConfigured {
		t.Fatalf("GetGeneralSetting().AIAgentConfigured = false, want true")
	}

	var persisted GeneralSetting
	if err := DB.First(&persisted, 1).Error; err != nil {
		t.Fatalf("load persisted general setting: %v", err)
	}
	if !persisted.AIAgentEnabled {
		t.Fatalf("persisted AIAgentEnabled = false, want true")
	}
	if !persisted.AIAgentConfigured {
		t.Fatalf("persisted AIAgentConfigured = false, want true")
	}
}

func TestUpdateGeneralSettingMarksAIAgentConfigured(t *testing.T) {
	updates := map[string]interface{}{
		"ai_agent_enabled": false,
	}

	markAIAgentConfigured(updates)

	if configured, ok := updates["ai_agent_configured"].(bool); !ok || !configured {
		t.Fatalf("ai_agent_configured = %#v, want true", updates["ai_agent_configured"])
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

func useTestGeneralSettingDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&GeneralSetting{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	DB = db
	t.Cleanup(func() {
		DB = originalDB
	})
}
