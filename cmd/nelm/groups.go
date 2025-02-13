package main

import "github.com/spf13/cobra"

var ReleaseGroup = &cobra.Group{
	ID:    "release",
	Title: "Release Commands:",
}

var ChartGroup = &cobra.Group{
	ID:    "chart",
	Title: "Chart Commands:",
}

var PlanGroup = &cobra.Group{
	ID:    "plan",
	Title: "Plan Commands:",
}
