package plan

import (
	"fmt"
	"io"
)

func NewDeployReportPrinter(outStream io.Writer, report *Report) *DeployReportPrinter {
	return &DeployReportPrinter{
		outStream: outStream,
		report:    report,
	}
}

type DeployReportPrinter struct {
	outStream io.Writer
	report    *Report
}

func (p *DeployReportPrinter) PrintSummary() {
	var createdResources []string
	for _, created := range p.report.Created {
		createdResources = append(createdResources, created.Target.String())
	}

	var updatedResources []string
	for _, updated := range p.report.Updated {
		updatedResources = append(updatedResources, updated.Target.String())
	}

	var deletedResources []string
	for _, deleted := range p.report.Deleted {
		deletedResources = append(deletedResources, deleted.String())
	}

	if p.report.Created != nil {
		fmt.Fprintf(p.outStream, "Resources created:\n")
		for _, created := range p.report.Created {
			fmt.Fprintf(p.outStream, "- %s\n", created.Target.String())
		}
	}

	if p.report.Recreated != nil {
		fmt.Fprintf(p.outStream, "Resources recreated:\n")
		for _, recreated := range p.report.Recreated {
			fmt.Fprintf(p.outStream, "- %s\n", recreated.Target.String())
		}
	}

	if p.report.Updated != nil {
		fmt.Fprintf(p.outStream, "Resources updated:\n")
		for _, updated := range p.report.Updated {
			fmt.Fprintf(p.outStream, "- %s\n", updated.Target.String())
		}
	}

	if p.report.Deleted != nil {
		fmt.Fprintf(p.outStream, "Resources deleted:\n")
		for _, deleted := range p.report.Deleted {
			fmt.Fprintf(p.outStream, "- %s\n", deleted.String())
		}
	}
}
