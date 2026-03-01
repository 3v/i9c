package agent

var AllToolDefs = []ToolDef{
	{
		Name:        "list_resources",
		Description: "List AWS resources of a specific type across profiles. Returns JSON with real resource IDs, ARNs, names, and properties.",
		Parameters: map[string]ToolParam{
			"resource_type": {Type: "string", Description: "AWS resource type (e.g., AWS::EC2::Instance, AWS::EC2::TransitGateway, AWS::EC2::VPC)"},
			"profile":       {Type: "string", Description: "AWS profile name. Leave empty to search all profiles."},
		},
		Required: []string{"resource_type"},
	},
	{
		Name:        "describe_resource",
		Description: "Get detailed information about a specific AWS resource by ID",
		Parameters: map[string]ToolParam{
			"resource_type": {Type: "string", Description: "AWS resource type"},
			"resource_id":   {Type: "string", Description: "Resource identifier (e.g., vpc-0abc123)"},
			"profile":       {Type: "string", Description: "AWS profile name"},
		},
		Required: []string{"resource_id"},
	},
	{
		Name:        "get_account_context",
		Description: "Fetch foundational AWS resources (VPCs, subnets, security groups, route tables, NAT gateways, internet gateways) for a profile. Returns real IDs needed when generating IaC code. Always call this before generating HCL that references existing infrastructure.",
		Parameters: map[string]ToolParam{
			"profile": {Type: "string", Description: "AWS profile name. Leave empty for the default profile."},
		},
		Required: []string{},
	},
	{
		Name:        "get_drift",
		Description: "Get current drift detection results between IaC definitions and actual AWS state",
		Parameters:  map[string]ToolParam{},
		Required:    []string{},
	},
	{
		Name:        "read_hcl_file",
		Description: "Read the contents of an HCL/Terraform/OpenTofu file from the IaC directory",
		Parameters: map[string]ToolParam{
			"path": {Type: "string", Description: "Filename relative to the IaC directory (e.g., main.tf)"},
		},
		Required: []string{"path"},
	},
	{
		Name:        "list_hcl_resources",
		Description: "List all resource, data, module, variable, and output definitions in the IaC directory",
		Parameters:  map[string]ToolParam{},
		Required:    []string{},
	},
	{
		Name:        "generate_hcl",
		Description: "Generate HCL code and write it to a file in the IaC directory. Use real resource IDs from get_account_context, never placeholders.",
		Parameters: map[string]ToolParam{
			"filename": {Type: "string", Description: "Output filename (e.g., transit_gateway.tf). Must not contain path separators."},
			"content":  {Type: "string", Description: "Complete HCL content to write to the file"},
		},
		Required: []string{"filename", "content"},
	},
	{
		Name:        "is_folder_empty",
		Description: "Check if the IaC directory contains any .tf or .tofu files",
		Parameters:  map[string]ToolParam{},
		Required:    []string{},
	},
}
