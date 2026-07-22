package main

func commandHelpCatalog() []commandHelp {
	return concatCommandHelpCatalogs(
		paneCommandHelpCatalog(),
		sessionCommandHelpCatalog(),
		agentCommandHelpCatalog(),
		loopCommandHelpCatalog(),
		workflowV2CommandHelpCatalog(),
		hookCommandHelpCatalog(),
		llmCommandHelpCatalog(),
		paneUtilityCommandHelpCatalog(),
	)
}

func concatCommandHelpCatalogs(groups ...[]commandHelp) []commandHelp {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	catalog := make([]commandHelp, 0, total)
	for _, group := range groups {
		catalog = append(catalog, group...)
	}
	return catalog
}
