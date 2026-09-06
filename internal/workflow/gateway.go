package workflow

import "github.com/alex-mccollum/igw-cli/internal/catalog"

const RestartOperation = "POST /data/api/v1/restart-tasks/restart"
const RestartTasksOperation = "GET /data/api/v1/restart-tasks/pending"
const OverviewOperation = "GET /data/api/v1/overview"
const RedundancyOperation = "GET /data/api/v1/redundancy"

func GatewayRestart() catalog.Capability {
	return catalog.Capability{ID: "gateway.restart.verified", Description: "Restart and observe a process change on the same Gateway node with no pending restart tasks", RequiredOperations: []string{RestartOperation, RestartTasksOperation, OverviewOperation, RedundancyOperation}}
}

// Assess includes current workflow prerequisites. Historical qualification
// continues to assess its original scopes, rather than claiming new coverage.
func Assess(c *catalog.Catalog) ([]catalog.CapabilityAssessment, error) {
	out, err := AssessTags(c)
	if err != nil {
		return nil, err
	}
	restart, err := c.Assess(GatewayRestart())
	if err != nil {
		return nil, err
	}
	return append(out, restart), nil
}
