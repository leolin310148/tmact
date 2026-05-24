package main

func commandHelpCatalog() []commandHelp {
	return concatCommandHelpCatalogs(
		paneCommandHelpCatalog(),
		agentCommandHelpCatalog(),
		workflowCommandHelpCatalog(),
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
