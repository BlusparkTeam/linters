package engine

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Consoneo/linters/src/config"
	"github.com/Consoneo/linters/src/rules"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/chelnak/ysmrr"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Analyse struct{}

type LintResult = rules.LintResult

func (o *Analyse) Lint() error {
	return o.executeLint("lint")
}

func (o *Analyse) Fix() error {
	return o.executeLint("fix")
}

func (o *Analyse) executeLint(lintType string) error {
	lintFile := ".linters.yaml"

	// check if the file exists
	if _, err := os.Stat(lintFile); os.IsNotExist(err) {
		fmt.Println("File .linters.yaml does not exist")
		os.Exit(1)
	}

	// Load the .linters.yaml file
	file, err := os.Open(lintFile)
	if err != nil {
		fmt.Println("Error opening file: ", err)
		os.Exit(1)
	}
	defer file.Close()

	// Unmarshal the yaml file
	var linters map[string]interface{}
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&linters)
	if err != nil {
		fmt.Println("Error decoding yaml: ", err)
		os.Exit(1)
	}

	// Ensure docker is installed
	_, err = rules.CheckIfDockerIsInstalled()
	if err != nil {
		fmt.Println("Docker is not installed")
		os.Exit(1)
	}

	// Ensure build dir is empty
	if os.RemoveAll("build") != nil {
		fmt.Println("Error removing build directory")
		os.Exit(1)
	}

	// Ensure build dir exists
	if _, err := os.Stat("build"); os.IsNotExist(err) {
		if err := os.Mkdir("build", 0755); err != nil {
			log.Error("Error creating build directory: ", err)
			os.Exit(1)
		}
	}

	// Start spinner
	sm := ysmrr.NewSpinnerManager()
	sm.Start()

	// Start channels
	var wg sync.WaitGroup
	ch := make(chan []LintResult)

	languages := []string{"php", "javascript", "css", "html"}

	for _, language := range languages {

		// check if language is configured
		if _, ok := linters["lints"].(map[interface{}]interface{})[language]; !ok {
			continue
		}

		yamlConfig := linters["lints"].(map[interface{}]interface{})[language]
		directories := yamlConfig.(map[interface{}]interface{})["src"].([]interface{})

		// if key version exists
		version := "latest"
		if _, ok := yamlConfig.(map[interface{}]interface{})["version"]; ok {
			version = yamlConfig.(map[interface{}]interface{})["version"].(string)
		}

		pattern := "*"
		switch language {
		case "php":
			pattern = "*.php"
		case "javascript":
			pattern = "*.js"
			if version == "latest" {
				version = "22"
			}
		case "css":
			pattern = "*.css"
		case "html":
			pattern = "*.html"
		}

		executionConfig := config.Config{
			Version: version,
			Pattern: pattern,
		}

		for _, dir := range directories {

			// if not rules, skip
			if _, ok := yamlConfig.(map[interface{}]interface{})["rules"]; !ok {
				continue
			}

			// get rules
			rulesToApply := yamlConfig.(map[interface{}]interface{})["rules"].([]interface{})

			availableRules := rules.Rules()
			for _, availableRule := range availableRules {

				for _, rule := range rulesToApply {

					// if not applyable (same slug, pass)
					if availableRule.Slug() != rule {
						continue
					}

					wg.Add(1)
					go func(rule interface{}) {
						defer wg.Done()

						spinner := sm.AddSpinner("(" + language + ") " + availableRule.Name())

						executionConfig.Path = dir.(string)

						// if path is not absolute, make it absolute
						if dir.(string)[0] != '/' {
							executionConfig.Path, _ = os.Getwd()
							executionConfig.Path += "/" + dir.(string)
						}

						var results []LintResult
						var err error

						if lintType == "lint" {
							// Use DetailedRule if available for rich output
							if detailedRule, ok := availableRule.(rules.DetailedRule); ok {
								results, err = detailedRule.ExecuteDetailed(executionConfig)
							} else {
								// Fallback for simple rules
								_, err = availableRule.Execute(executionConfig)
								if err != nil {
									results = append(results, LintResult{
										Rule:     availableRule.Slug(),
										File:     executionConfig.Path,
										Line:     0,
										Message:  err.Error(),
										Severity: "error",
									})
								}
							}
						} else {
							if availableRule.CanFix() {
								_, err = availableRule.Fix(executionConfig)
							} else {
								spinner.UpdateMessage("skipped: " + availableRule.Name())
								ch <- []LintResult{}
								return
							}
						}

						if err != nil {
							spinner.Error()
						} else {
							spinner.Complete()
						}

						ch <- results
					}(rule)
				}
			}

			// custom commands
			if _, ok := yamlConfig.(map[interface{}]interface{})["commands"]; ok {
				commands := yamlConfig.(map[interface{}]interface{})["commands"].([]interface{})
				for _, command := range commands {
					wg.Add(1)
					go func(command interface{}) {
						defer wg.Done()

						spinner := sm.AddSpinner("(" + language + ") " + command.(string))

						customCommand := rules.CustomCommand{Command: command.(string)}
						_, err := customCommand.Execute()
						if err != nil {
							spinner.Error()
						} else {
							spinner.Complete()
						}

						var results []LintResult
						if err != nil {
							results = append(results, LintResult{
								Rule:     "custom-command",
								File:     command.(string),
								Line:     0,
								Message:  err.Error(),
								Severity: "error",
							})
						}
						ch <- results
					}(command)
				}
			}
		}
	}

	go func() {
		wg.Wait()
		close(ch)
		sm.Stop()
	}()

	// Collect all results
	allResults := []LintResult{}
	for results := range ch {
		allResults = append(allResults, results...)
	}

	if len(allResults) > 0 {
		o.displaySummary(allResults)
		return errors.New("linting failed")
	}

	return nil
}

func (o *Analyse) displaySummary(allResults []LintResult) {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		MarginTop(1)

	fmt.Println(style.Render("\n✗ Linting Violations"))

	// Group by file
	fileViolations := make(map[string][]LintResult)
	for _, r := range allResults {
		fileViolations[r.File] = append(fileViolations[r.File], r)
	}

	// Display per file
	for file, violations := range fileViolations {
		fileStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true).
			MarginTop(1)

		fmt.Printf("\n%s (%d issues)\n", fileStyle.Render(file), len(violations))

		for _, v := range violations {
			icon := "⚠"
			color := "214"
			if v.Severity == "error" {
				icon = "✗"
				color = "196"
			}

			location := ""
			if v.Line > 0 {
				location = fmt.Sprintf(":%d", v.Line)
			}

			msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
			fmt.Println(msgStyle.Render(fmt.Sprintf("  %s Line%s [%s] %s", icon, location, v.Rule, v.Message)))
		}
	}

	// Summary line
	summaryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		MarginTop(1)
	fmt.Println(summaryStyle.Render(fmt.Sprintf("\n✗ %d violation(s) found", len(allResults))))
}

func (o *Analyse) InitConfig() {
	// create a .linters.yaml file with basic configuration
	lintFile := ".linters.yaml"
	if _, err := os.Stat(lintFile); !os.IsNotExist(err) {
		fmt.Println(".linters.yaml already exists")
		os.Exit(1)
	}

	file, err := os.Create(lintFile)
	if err != nil {
		fmt.Println("Error creating file: ", err)
		os.Exit(1)
	}

	defer file.Close()

	template := `
lints:
  php:
    version: "8.4"
    src:
      - src
      - src2
    rules:
      - no-syntax-error
      - no-dump
      - no-exit
      - psr1
      - psr2
	commands:
	  - bash mycustomcommand.sh
      
  javascript:
    version: "8.1"
    src:
      - ./front/
    rules:
      - eslint
`

	_, err = file.WriteString(template)
	if err != nil {
		fmt.Println("Error writing to file: ", err)
		os.Exit(1)
	}

	fmt.Println("File .linters.yaml created")
}

func (o *Analyse) Install(eventName string) {

	// Ensure current dir is a git repository
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		fmt.Println("Not a git repository")
		os.Exit(1)
	}

	// create a pre-commit hook
	hookFile := ".git/hooks/"+eventName
	if _, err := os.Stat(hookFile); !os.IsNotExist(err) {
		fmt.Println(eventName+" hook already exists")
		os.Exit(1)
	}

	file, err := os.Create(hookFile)
	if err != nil {
		fmt.Println("Error creating file: ", err)
		os.Exit(1)
	}

	defer file.Close()

	template := `#!/bin/sh
	linters lint
	`

	_, err = file.WriteString(template)
	if err != nil {
		fmt.Println("Error writing to file: ", err)
		os.Exit(1)
	}

	err = os.Chmod(hookFile, 0755)
	if err != nil {
		fmt.Println("Error changing permissions: ", err)
		os.Exit(1)
	}

	fmt.Println(eventName+" hook created")
}

func (o *Analyse) ListRules() {

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
		Headers("RULE", "DESCRIPTION", "AUTO FIX")

	availableRules := rules.Rules()
	yesText := lipgloss.NewStyle().Foreground(lipgloss.Color("118")).Render("yes")
	noText := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("no")
	for _, availableRule := range availableRules {
		fixable := noText
		if availableRule.CanFix() {
			fixable = yesText
		}
		t.Row(availableRule.Slug(), availableRule.Name(), fixable)
	}

	fmt.Println(t.Render())
}

func (o *Analyse) ListReports() {
	// List reports in the build folder
	reports := []string{}
	folders, err := os.ReadDir("build")
	if err != nil {
		fmt.Println("Error reading build directory: ", err)
		os.Exit(1)
	}

	for _, folder := range folders {
		if folder.IsDir() {
			reports = append(reports, folder.Name())
		}
	}

	if len(reports) > 0 {

		style := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).MarginTop(2)
		fmt.Fprintf(os.Stdout, "%s\n", style.Render("Some reports were generated"))

		t := table.New().
			Border(lipgloss.NormalBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
			Headers("REPORTS")

		for _, report := range reports {
			t.Row("./build/" + report)
		}

		fmt.Println(t.Render())
	}
}
