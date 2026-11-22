package home

import (
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var templatePlaces = []string{"{branch}", "{base}", "{commit}", "{request}", "{title}", "{summary}"}

func templatesCreate(forge string, templates *proto.ForgeTemplates) *Refusal {
	if (forge == "custom") != (templates != nil) {
		return &Refusal{"bad_request", "templates are required exactly for a custom forge"}
	}
	return templatesValid(templates)
}

func templatesSet(forge string, templates *proto.ForgeTemplates) *Refusal {
	if templates == nil {
		return nil
	}
	return templatesCreate(forge, templates)
}

func templatesValid(t *proto.ForgeTemplates) *Refusal {
	if t == nil {
		return nil
	}
	for name, args := range map[string][]string{"push": t.Push, "open": t.Open, "checks": t.Checks, "merge": t.Merge} {
		if !templateValid(name, args) {
			return &Refusal{"bad_request", "invalid forge template: " + name}
		}
	}
	return nil
}

func templateValid(name string, args []string) bool {
	if len(args) == 0 || args[0] == "" || strings.ContainsAny(args[0], `/\`) {
		return false
	}
	for i, arg := range args {
		if !templateArgument(name, arg, i == 0) {
			return false
		}
	}
	return true
}

func templateArgument(name, arg string, program bool) bool {
	if strings.HasPrefix(arg, "{title}") || strings.HasPrefix(arg, "{summary}") {
		return false
	}
	for _, place := range templatePlaces {
		if strings.Contains(arg, place) && !templateAllows(name, place, program) {
			return false
		}
	}
	return true
}

func templateAllows(name, place string, program bool) bool {
	if program {
		return false
	}
	switch place {
	case "{request}":
		return name == "checks" || name == "merge"
	case "{title}", "{summary}":
		return name == "open"
	default:
		return true
	}
}
