package mission

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const MaxReplicas = 3

var workloadName = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]+)*$")

func ValidateWorkloads(items []Workload) error {
	if len(items) < 1 || len(items) > 4 {
		return fmt.Errorf("declare between one and four Kubernetes workloads")
	}
	names := map[string]bool{}
	for _, w := range items {
		if !workloadName.MatchString(w.Name) || len(w.Name) > 40 || names[w.Name] {
			return fmt.Errorf("invalid or duplicate workload name %q", w.Name)
		}
		names[w.Name] = true
		if w.Kind != "Pod" && w.Kind != "Deployment" {
			return fmt.Errorf("unsupported workload kind %q", w.Kind)
		}
		if !workloadName.MatchString(w.App) || len(w.App) > 40 {
			return fmt.Errorf("invalid app label %q", w.App)
		}
		if strings.TrimSpace(w.Log) == "" || len(w.Log) > 256 || strings.IndexFunc(w.Log, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
			return fmt.Errorf("expected one printable fixture log line")
		}
	}
	return nil
}

func (m Mission) HasWorkload(kind, name string) bool {
	for _, w := range m.Kubernetes {
		if w.Kind == kind && w.Name == name {
			return true
		}
	}
	return false
}
