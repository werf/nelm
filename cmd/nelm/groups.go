package main

import "github.com/spf13/cobra"

var releaseGroup = &cobra.Group{
	ID:    "release",
	Title: "Release commands:",
}

var chartGroup = &cobra.Group{
	ID:    "chart",
	Title: "Chart commands:",
}

var planGroup = &cobra.Group{
	ID:    "plan",
	Title: "Plan commands:",
}
