package rules

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/Consoneo/linters/src/config"
)

type Psr12 struct {
}

func (o *Psr12) ExecuteDetailed(c config.Config) ([]LintResult, error) {
	cmd := exec.Command(
		"docker", "run", "--rm",
		"-v", c.Path+":/app",
		"ghcr.io/php-cs-fixer/php-cs-fixer:3-php"+c.Version,
		"fix", "/app",
		"--rules=@PSR12",
		"--dry-run",
		"--format=json",
		"--diff",
		"--using-cache=no",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		return []LintResult{}, nil
	}

	results := parseFixerJsonOutput(stdout.Bytes(), stderr.Bytes(), o.Slug(), "PSR-12", c.Path)

	if len(results) > 0 {
		return results, fmt.Errorf("PSR-12 violations found")
	}

	return results, nil
}

func (o *Psr12) Execute(c config.Config) (string, error) {
	results, err := o.ExecuteDetailed(c)
	if err != nil {
		return fmt.Sprintf("%d violations", len(results)), err
	}
	return "", nil
}

func (o *Psr12) Name() string {
	return "Check for PSR12 compliance"
}

func (o *Psr12) Slug() string {
	return "psr12"
}

func (o *Psr12) CanFix() bool {
	return true
}

func (o *Psr12) Fix(config config.Config) (string, error) {
	command := "docker run --rm -v " + config.Path + ":/code ghcr.io/php-cs-fixer/php-cs-fixer:${FIXER_VERSION:-3-php" + config.Version + "} fix --rules=@PSR12 ."
	return ExecuteCommandAndExpectNoResultToBeCorrect(command)
}

