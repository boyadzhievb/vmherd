package config

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxKeySuggestionDistance is the largest number of single-character edits
// between an unknown key and a known one for vmherd to suggest the known key.
const maxKeySuggestionDistance = 2

// unknownFieldPattern matches yaml.v3's KnownFields message, for example
// "line 3: field memroy not found in type config.VM".
var unknownFieldPattern = regexp.MustCompile(`^line (\d+): field (\S+) not found in type config\.(\w+)$`)

// typeMismatchPattern matches yaml.v3's type error, for example
// "line 4: cannot unmarshal !!str `two` into int".
var typeMismatchPattern = regexp.MustCompile("^line (\\d+): cannot unmarshal !!(\\w+) `(.*)` into (.+)$")

// herdfileSection describes a Herdfile struct type in the user's terms.
type herdfileSection struct {
	location   string       // where the key was found, e.g. "in a VM (under 'vms:')"
	structType reflect.Type // the struct whose yaml tags are the known keys
}

// herdfileSections maps the Go type names yaml.v3 reports to Herdfile sections.
func herdfileSections() map[string]herdfileSection {
	return map[string]herdfileSection{
		"Herdfile":        {"at the top level", reflect.TypeOf(Herdfile{})},
		"VM":              {"in a VM (under 'vms:')", reflect.TypeOf(VM{})},
		"VMNetworkConfig": {"in a VM's 'network:'", reflect.TypeOf(VMNetworkConfig{})},
		"SharedFolder":    {"in a 'shared_folders:' entry", reflect.TypeOf(SharedFolder{})},
		"PortForward":     {"in a 'ports:' entry", reflect.TypeOf(PortForward{})},
		"Provision":       {"in a 'provision:' step", reflect.TypeOf(Provision{})},
		"NetworkConfig":   {"in the top-level 'network:'", reflect.TypeOf(NetworkConfig{})},
		"SSHConfig":       {"in 'ssh:'", reflect.TypeOf(SSHConfig{})},
	}
}

// describeYAMLDecodeError turns a yaml.v3 decode error into Herdfile problems:
// one per unknown key or wrongly typed value, each with its line number. A
// syntax error (no TypeError) is returned as a single problem.
func describeYAMLDecodeError(decodeErr error) []error {
	var typeError *yaml.TypeError
	if !errors.As(decodeErr, &typeError) {
		return []error{fmt.Errorf("invalid YAML syntax: %w", decodeErr)}
	}
	problems := make([]error, 0, len(typeError.Errors))
	for _, message := range typeError.Errors {
		problems = append(problems, errors.New(describeYAMLMessage(message)))
	}
	return problems
}

// describeYAMLMessage rewrites one yaml.v3 message in Herdfile terms, or
// returns it unchanged if it has an unexpected form.
func describeYAMLMessage(message string) string {
	if match := unknownFieldPattern.FindStringSubmatch(message); match != nil {
		return describeUnknownKey(match[1], match[2], match[3])
	}
	if match := typeMismatchPattern.FindStringSubmatch(message); match != nil {
		return fmt.Sprintf("line %s: %s is %s, but this key needs %s",
			match[1], describeYAMLValue(match[2], match[3]), describeYAMLKind(match[2]), describeGoType(match[4]))
	}
	return message
}

// describeUnknownKey explains an unknown key: where it is, the closest known
// key if it looks like a typo, otherwise all keys allowed there.
func describeUnknownKey(lineNumber, unknownKey, goTypeName string) string {
	section, known := herdfileSections()[goTypeName]
	if !known {
		return fmt.Sprintf("line %s: unknown key '%s' — check the spelling", lineNumber, unknownKey)
	}
	knownKeys := yamlKeys(section.structType)
	if suggestion := closestKey(unknownKey, knownKeys); suggestion != "" {
		return fmt.Sprintf("line %s: unknown key '%s' %s — did you mean '%s'?", lineNumber, unknownKey, section.location, suggestion)
	}
	return fmt.Sprintf("line %s: unknown key '%s' %s — known keys there: %s",
		lineNumber, unknownKey, section.location, strings.Join(knownKeys, ", "))
}

// yamlKeys returns the sorted yaml keys of a struct type (fields tagged "-"
// are internal and left out).
func yamlKeys(structType reflect.Type) []string {
	var keys []string
	for fieldIndex := range structType.NumField() {
		tagName, _, _ := strings.Cut(structType.Field(fieldIndex).Tag.Get("yaml"), ",")
		if tagName != "" && tagName != "-" {
			keys = append(keys, tagName)
		}
	}
	sort.Strings(keys)
	return keys
}

// closestKey returns the known key nearest to unknownKey when it is within
// maxKeySuggestionDistance edits (and the edits don't make up the whole key).
func closestKey(unknownKey string, knownKeys []string) string {
	bestKey, bestDistance := "", maxKeySuggestionDistance+1
	for _, knownKey := range knownKeys {
		distance := editDistance(strings.ToLower(unknownKey), knownKey)
		if distance < bestDistance && distance < len(knownKey) {
			bestKey, bestDistance = knownKey, distance
		}
	}
	return bestKey
}

// editDistance is the Damerau-Levenshtein (optimal string alignment) distance:
// insertions, deletions, substitutions and swaps of neighbours each count 1.
func editDistance(first, second string) int {
	firstRunes, secondRunes := []rune(first), []rune(second)
	distances := make([][]int, len(firstRunes)+1)
	for row := range distances {
		distances[row] = make([]int, len(secondRunes)+1)
		distances[row][0] = row
	}
	for column := range secondRunes {
		distances[0][column+1] = column + 1
	}
	for row := 1; row <= len(firstRunes); row++ {
		for column := 1; column <= len(secondRunes); column++ {
			substitutionCost := 1
			if firstRunes[row-1] == secondRunes[column-1] {
				substitutionCost = 0
			}
			distances[row][column] = min(distances[row-1][column]+1, distances[row][column-1]+1,
				distances[row-1][column-1]+substitutionCost)
			if row > 1 && column > 1 && firstRunes[row-1] == secondRunes[column-2] && firstRunes[row-2] == secondRunes[column-1] {
				distances[row][column] = min(distances[row][column], distances[row-2][column-2]+1)
			}
		}
	}
	return distances[len(firstRunes)][len(secondRunes)]
}

// describeYAMLValue quotes a scalar value; lists and mappings have no short form.
func describeYAMLValue(yamlKind, value string) string {
	if yamlKind == "seq" || yamlKind == "map" {
		return "the value"
	}
	return fmt.Sprintf("'%s'", value)
}

// describeYAMLKind names a YAML node kind (yaml.v3's !!str, !!seq, …).
func describeYAMLKind(yamlKind string) string {
	switch yamlKind {
	case "str":
		return "text"
	case "int":
		return "a number"
	case "float":
		return "a decimal number"
	case "bool":
		return "true/false"
	case "seq":
		return "a list"
	case "map":
		return "a mapping (key: value lines)"
	case "null":
		return "empty"
	default:
		return "a " + yamlKind
	}
}

// describeGoType names the kind of value a Herdfile field expects.
func describeGoType(goType string) string {
	switch {
	case goType == "int":
		return "a whole number"
	case goType == "bool":
		return "true or false"
	case goType == "string" || strings.HasSuffix(goType, "Type"):
		return "text"
	case strings.HasPrefix(goType, "[]"):
		return "a list (lines starting with '- ')"
	default:
		return "a mapping (key: value lines)"
	}
}
