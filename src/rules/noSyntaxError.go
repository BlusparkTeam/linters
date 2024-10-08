package rules

import "github.com/Consoneo/linters/src/config"

type NoSyntaxError struct {
}

func (o *NoSyntaxError) Execute(config config.Config) (string, error) {
	command := "docker run --rm -v " + config.Path + ":/data php:" + config.Version + "-cli find /data -name '*.php' -exec php -l {} \\;|grep -v 'No syntax errors'|wc -l"
	return ExecuteCommandAndExpectNoResultToBeCorrect(command)
}

func (o *NoSyntaxError) Name() string {
	return "Check for syntax errors in PHP files"
}

func (o *NoSyntaxError) Slug() string {
	return "no-syntax-error"
}

func (o *NoSyntaxError) CanFix() bool {
	return false
}

func (o *NoSyntaxError) Fix(config config.Config) (string, error) {
	return "", nil
}

