package testdb

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestEnvVars(t *testing.T) {
	b, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)

	b, err = os.ReadFile(".envrc")
	if err != nil {
		// on CI, .envrc does not exist
		if os.IsNotExist(err) {
			t.Skip(".envrc file does not exist")
		}
		t.Fatal(err)
	}
	envrc := string(b)

	// test that all exported variables in .envrc are documented in the README
	// with the appropriate format, e.g.:
	// * `ENV_VAR=non-secret value or description of secret value`
	t.Run("envrc", func(t *testing.T) {
		rxExport := regexp.MustCompile(`^export (\w+)=`)

		s := bufio.NewScanner(strings.NewReader(envrc))
		for s.Scan() {
			line := s.Text()
			ms := rxExport.FindStringSubmatch(line)
			if ms != nil {
				if !strings.Contains(readme, fmt.Sprintf("* `%s=", ms[1])) {
					t.Errorf("environment variable %q not documented in README file", ms[1])
				}
			}
		}
		if err := s.Err(); err != nil {
			t.Fatal(err)
		}
	})

	// test that all documented variables in the README exist in the .envrc
	t.Run("readme", func(t *testing.T) {
		rxDoc := regexp.MustCompile("^\\* `(\\w+)=")

		s := bufio.NewScanner(strings.NewReader(readme))
		for s.Scan() {
			line := s.Text()
			ms := rxDoc.FindStringSubmatch(line)
			if ms != nil {
				if !strings.Contains(envrc, fmt.Sprintf("export %s=", ms[1])) {
					t.Errorf("documented variable %q not exported in .envrc file", ms[1])
				}
			}
		}
		if err := s.Err(); err != nil {
			t.Fatal(err)
		}
	})
}
