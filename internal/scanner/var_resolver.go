package scanner

import "regexp"

// stringVarDef matches string constant/variable definitions.
type stringVarDef struct {
	re       *regexp.Regexp
	nameGrp  int
	valueGrp int
}

// stringVarDefs extract string constant/variable assignments from source code.
var stringVarDefs = []stringVarDef{
	// Go: const x = "val" or var x = "val" (optional type annotation)
	{regexp.MustCompile(`(?:const|var)\s+([a-zA-Z_]\w*)\s+(?:string\s*)?=\s*"([^"]+)"`), 1, 2},
	// Go: const x = "val" (no type, just assignment)
	{regexp.MustCompile(`(?:const|var)\s+([a-zA-Z_]\w*)\s*=\s*"([^"]+)"`), 1, 2},
	// JS/TS: const x = "val" or let x = "val" or var x = "val"
	{regexp.MustCompile(`(?:const|let|var)\s+([a-zA-Z_]\w*)\s*=\s*["']([^"']+)["']`), 1, 2},
	// Python: x = "val" (module-level string assignment)
	{regexp.MustCompile(`^([a-zA-Z_]\w*)\s*=\s*["']([^"']+)["']`), 1, 2},
}

// varCollRef matches collection calls with variable (non-literal) arguments.
type varCollRef struct {
	re    *regexp.Regexp
	group int
}

// varCollectionRefs detect collection calls that use variable names instead of literals.
var varCollectionRefs = []varCollRef{
	// Go: .Collection(varName)
	{regexp.MustCompile(`\.Collection\(\s*([a-zA-Z_]\w*)\s*,?\s*\)`), 1},
	// JS/TS: .collection(varName), .getCollection(varName)
	{regexp.MustCompile(`\.(?:collection|getCollection|GetCollection)\(\s*([a-zA-Z_]\w*)\s*,?\s*\)`), 1},
}

// collectStringVars scans joined lines for string constant/variable definitions
// and returns a map of variable name to string value.
func collectStringVars(lines []joinedLine) map[string]string {
	vars := make(map[string]string)
	for _, jl := range lines {
		for _, def := range stringVarDefs {
			for _, m := range def.re.FindAllStringSubmatch(jl.text, -1) {
				name := m[def.nameGrp]
				value := m[def.valueGrp]
				vars[name] = value
			}
		}
	}
	return vars
}

// resolveVarCollections finds collection calls that use variable arguments
// and resolves them against known string variables.
// Returns resolved collection matches and unresolvable variable names.
func resolveVarCollections(line string, vars map[string]string) (resolved []match, dynamic []string) {
	for _, p := range varCollectionRefs {
		for _, m := range p.re.FindAllStringSubmatch(line, -1) {
			varName := m[p.group]
			if value, ok := vars[varName]; ok {
				if isValidCollectionName(value) {
					resolved = append(resolved, match{
						Collection: value,
						Pattern:    PatternDriverCall,
					})
				}
			} else {
				dynamic = append(dynamic, varName)
			}
		}
	}
	return
}
