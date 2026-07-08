package cmd

// Process exit codes returned by Execute.
const (
	// codeOK indicates success.
	codeOK = 0
	// codeRuntime indicates a runtime failure.
	codeRuntime = 1
	// codeUsage indicates a usage or flag error.
	codeUsage = 2
)
