package ai

func helmToolDefinitions() []agentToolDefinition {
	return []agentToolDefinition{
		{
			Name:        "list_helm_releases",
			Description: "List Helm releases in a namespace, or across namespaces when the user has cluster-wide Helm release read access.",
			Properties: map[string]any{
				"namespace": helmNamespaceSchema("Namespace to list releases in. Leave empty only when the user asks for all accessible releases."),
			},
		},
		{
			Name:        "get_helm_release",
			Description: "Get a Helm release by release name and namespace. Returns release metadata, values, status, and rendered resource summary.",
			Properties: map[string]any{
				"release_name": helmReleaseNameSchema(),
				"namespace":    helmNamespaceSchema("Namespace containing the release."),
			},
			Required: []string{"release_name", "namespace"},
		},
		{
			Name:        "get_helm_release_history",
			Description: "Get revision history for a Helm release.",
			Properties: map[string]any{
				"release_name": helmReleaseNameSchema(),
				"namespace":    helmNamespaceSchema("Namespace containing the release."),
			},
			Required: []string{"release_name", "namespace"},
		},
		{
			Name:        "dry_run_install_helm_release",
			Description: "Render and validate a Helm install without changing the cluster. Always use this before install_helm_release so the user can review the resources.",
			Properties:  helmInstallProperties(),
			Required:    []string{"release_name", "namespace"},
		},
		{
			Name:        "install_helm_release",
			Description: "Install a Helm release after the user has reviewed a dry_run_install_helm_release result. This modifies cluster state and requires confirmation.",
			Properties:  helmInstallProperties(),
			Required:    []string{"release_name", "namespace"},
		},
		{
			Name:        "dry_run_upgrade_helm_release",
			Description: "Render and validate a Helm upgrade without changing the cluster. Always use this before upgrade_helm_release so the user can review the diff summary.",
			Properties:  helmUpgradeProperties(),
			Required:    []string{"release_name", "namespace"},
		},
		{
			Name:        "upgrade_helm_release",
			Description: "Upgrade an existing Helm release after the user has reviewed a dry_run_upgrade_helm_release result. This modifies cluster state and requires confirmation.",
			Properties:  helmUpgradeProperties(),
			Required:    []string{"release_name", "namespace"},
		},
		{
			Name:        "rollback_helm_release",
			Description: "Rollback a Helm release to a previous revision. This modifies cluster state and requires confirmation.",
			Properties: map[string]any{
				"release_name": helmReleaseNameSchema(),
				"namespace":    helmNamespaceSchema("Namespace containing the release."),
				"revision": map[string]any{
					"type":        "integer",
					"description": "Target revision. Leave empty to roll back to the previous revision.",
				},
			},
			Required: []string{"release_name", "namespace"},
		},
		{
			Name:        "uninstall_helm_release",
			Description: "Uninstall a Helm release. This modifies cluster state and requires confirmation.",
			Properties: map[string]any{
				"release_name": helmReleaseNameSchema(),
				"namespace":    helmNamespaceSchema("Namespace containing the release."),
				"description": map[string]any{
					"type":        "string",
					"description": "Optional operation description recorded in Helm history.",
				},
			},
			Required: []string{"release_name", "namespace"},
		},
	}
}

func helmInstallProperties() map[string]any {
	return map[string]any{
		"release_name":    helmReleaseNameSchema(),
		"namespace":       helmNamespaceSchema("Namespace to install into."),
		"source":          helmChartSourceSchema(),
		"repository_name": helmRepositoryNameSchema(),
		"chart_name":      helmChartNameSchema(),
		"chart_version":   helmChartVersionSchema(),
		"chart_url":       helmChartURLSchema(),
		"values":          helmValuesSchema(),
		"description": map[string]any{
			"type":        "string",
			"description": "Optional operation description recorded in Helm history.",
		},
		"create_namespace": map[string]any{
			"type":        "boolean",
			"description": "Whether Helm may create the target namespace. This requires namespace create permission and is blocked in namespace-scoped workspaces.",
		},
		"wait": map[string]any{
			"type":        "boolean",
			"description": "Whether to wait for rendered resources to become ready.",
		},
	}
}

func helmUpgradeProperties() map[string]any {
	return map[string]any{
		"release_name":        helmReleaseNameSchema(),
		"namespace":           helmNamespaceSchema("Namespace containing the release."),
		"source":              helmChartSourceSchema(),
		"repository_name":     helmRepositoryNameSchema(),
		"chart_name":          helmChartNameSchema(),
		"chart_version":       helmChartVersionSchema(),
		"chart_url":           helmChartURLSchema(),
		"values":              helmValuesSchema(),
		"force_conflicts":     helmBoolSchema("Whether to force server-side apply conflicts during upgrade."),
		"rollback_on_failure": helmBoolSchema("Whether to roll back automatically if the upgrade fails."),
		"wait":                helmBoolSchema("Whether to wait for rendered resources to become ready."),
		"description": map[string]any{
			"type":        "string",
			"description": "Optional operation description recorded in Helm history.",
		},
	}
}

func helmReleaseNameSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Helm release name.",
	}
}

func helmNamespaceSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func helmChartSourceSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Chart source. Use repository for Kite-managed chart repositories, or artifacthub for Artifact Hub charts. Defaults to repository.",
		"enum":        []string{"repository", "artifacthub"},
	}
}

func helmRepositoryNameSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Kite chart repository name, or Artifact Hub repository name when source is artifacthub.",
	}
}

func helmChartNameSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Chart name. Provide this with repository_name, or provide chart_url when using a known repository package URL.",
	}
}

func helmChartVersionSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Optional chart version. Leave empty for the latest matching version when supported.",
	}
}

func helmChartURLSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Resolved chart package URL. For repository charts, this must match the stored repository package URL.",
	}
}

func helmValuesSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          "Helm values object to pass to the chart.",
		"additionalProperties": true,
	}
}

func helmBoolSchema(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
	}
}
