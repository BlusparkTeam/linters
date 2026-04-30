package rules

import "github.com/Consoneo/linters/src/config"

type Rule interface {
	Slug() string

	Name() string

	Execute(config config.Config) (string, error)

	CanFix() bool

	Fix(config config.Config) (string, error)
}

// DetailedRule extends Rule with detailed lint results (file, line, message)
type DetailedRule interface {
	Rule
	ExecuteDetailed(config.Config) ([]LintResult, error)
}

// LintResult represents a single violation found by a rule
type LintResult struct {
	Rule     string
	File     string
	Line     int
	Column   int
	Message  string
	Severity string
}

