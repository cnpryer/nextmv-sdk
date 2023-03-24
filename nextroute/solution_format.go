package nextroute

import (
	"github.com/nextmv-io/sdk/alns"
	"github.com/nextmv-io/sdk/connect"
)

// The Formatter interface is used to create custom JSON output.
type Formatter interface {
	ToOutput(Solution) any
}

// NewBasicFormatter creates a new NewBasicFormatter.
func NewBasicFormatter() Formatter {
	connect.Connect(con, &newDefaultFormatter)
	return newDefaultFormatter()
}

// NewVerboseFormatter creates a NewVerboseFormatter which outputs
// additional solution information.
func NewVerboseFormatter(p []alns.ProgressionEntry) Formatter {
	connect.Connect(con, &newExtensiveFormatter)
	return newExtensiveFormatter(p)
}
