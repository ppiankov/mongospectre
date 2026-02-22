package analyzer

import (
	"fmt"
	"sort"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func detectValidatorDrift(scan *scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
	if len(scan.WriteRefs) == 0 {
		return nil
	}

	writtenCollections := make(map[string]bool)
	writeFields := make(map[string]map[string]map[string]bool)
	for _, wr := range scan.WriteRefs {
		coll := strings.ToLower(strings.TrimSpace(wr.Collection))
		if coll == "" {
			continue
		}
		writtenCollections[coll] = true

		field := strings.TrimSpace(wr.Field)
		if field == "" {
			continue
		}
		if writeFields[coll] == nil {
			writeFields[coll] = make(map[string]map[string]bool)
		}
		if writeFields[coll][field] == nil {
			writeFields[coll][field] = make(map[string]bool)
		}
		valueType := strings.TrimSpace(wr.ValueType)
		if valueType == "" {
			valueType = scanner.ValueTypeUnknown
		}
		writeFields[coll][field][valueType] = true
	}

	var findings []Finding
	for _, collName := range sortedBoolKeys(writtenCollections) {
		coll, found := findCollection(collName, collections)
		if !found || coll.Type == "view" {
			continue
		}
		if coll.Validator == nil {
			findings = append(findings, Finding{
				Type:       FindingValidatorMissing,
				Severity:   SeverityMedium,
				Database:   coll.Database,
				Collection: coll.Name,
				Message:    fmt.Sprintf("collection %q is written in code but has no JSON schema validator", coll.Name),
			})
			continue
		}

		validator := coll.Validator
		action := strings.ToLower(strings.TrimSpace(validator.ValidationAction))
		level := strings.ToLower(strings.TrimSpace(validator.ValidationLevel))
		if action == "" {
			action = "error"
		}
		if level == "" {
			level = "strict"
		}
		strictEnforced := action == "error" && level == "strict"

		if strictEnforced {
			findings = append(findings, Finding{
				Type:       FindingValidatorStrictRisk,
				Severity:   SeverityLow,
				Database:   coll.Database,
				Collection: coll.Name,
				Message:    "validator is in strict/error mode; schema mismatches will reject writes",
			})
		}
		if action == "warn" {
			findings = append(findings, Finding{
				Type:       FindingValidatorWarnOnly,
				Severity:   SeverityInfo,
				Database:   coll.Database,
				Collection: coll.Name,
				Message:    "validator action is warn; schema violations are logged but not rejected",
			})
		}

		props := validator.Schema.Properties
		if len(props) == 0 {
			continue
		}
		additionalPropsFalse := validator.Schema.AdditionalProperties != nil && !*validator.Schema.AdditionalProperties

		for _, field := range sortedWriteFieldKeys(writeFields[collName]) {
			if field == "_id" {
				continue
			}
			observed := writeFields[collName][field]
			schemaField, ok := props[field]
			if !ok {
				if additionalPropsFalse {
					severity := SeverityMedium
					if strictEnforced {
						severity = SeverityHigh
					}
					findings = append(findings, Finding{
						Type:       FindingFieldNotInValidator,
						Severity:   severity,
						Database:   coll.Database,
						Collection: coll.Name,
						Message:    fmt.Sprintf("field %q is written in code but missing from validator properties while additionalProperties=false", field),
					})
				}
				continue
			}

			allowedTypes := normalizeAllowedTypes(schemaField.BSONTypes)
			if len(allowedTypes) == 0 {
				continue
			}
			mismatched := mismatchedObservedTypes(observed, allowedTypes)
			if len(mismatched) == 0 {
				continue
			}

			severity := SeverityMedium
			if strictEnforced {
				severity = SeverityHigh
			}
			findings = append(findings, Finding{
				Type:       FindingValidatorStale,
				Severity:   severity,
				Database:   coll.Database,
				Collection: coll.Name,
				Message: fmt.Sprintf("validator expects field %q type [%s] but code writes [%s]",
					field, strings.Join(mapKeysSorted(allowedTypes), ", "), strings.Join(mismatched, ", ")),
			})
		}
	}

	return findings
}

func normalizeAllowedTypes(raw []string) map[string]bool {
	out := make(map[string]bool)
	for _, t := range raw {
		t = strings.TrimSpace(strings.ToLower(t))
		switch t {
		case "":
			continue
		case "int", "long", "double", "decimal":
			out[scanner.ValueTypeNumber] = true
		case "boolean":
			out[scanner.ValueTypeBool] = true
		case "objectid":
			out[scanner.ValueTypeObjectID] = true
		default:
			out[t] = true
		}
	}
	return out
}

func mismatchedObservedTypes(observed map[string]bool, allowed map[string]bool) []string {
	var mismatched []string
	for t := range observed {
		if t == "" || t == scanner.ValueTypeUnknown {
			continue
		}
		if !allowed[t] {
			mismatched = append(mismatched, t)
		}
	}
	sort.Strings(mismatched)
	return mismatched
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedWriteFieldKeys(m map[string]map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func mapKeysSorted(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
