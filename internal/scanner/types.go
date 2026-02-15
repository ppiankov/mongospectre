package scanner

// PatternType identifies how a collection reference was detected.
type PatternType string

const (
	PatternDriverCall PatternType = "driver_call" // db.Collection("x"), db.collection("x"), etc.
	PatternBracket    PatternType = "bracket"     // db["x"]
	PatternORM        PatternType = "orm"         // mongoose.model, MongoEngine
	PatternDotAccess  PatternType = "dot_access"  // db.users.find(...)
)

// CollectionRef represents a collection name found in source code.
type CollectionRef struct {
	Collection string      `json:"collection"`
	File       string      `json:"file"`
	Line       int         `json:"line"`
	Pattern    PatternType `json:"pattern"`
}

// FieldRef represents a queried field found in source code, tied to a collection.
type FieldRef struct {
	Collection string `json:"collection"`
	Field      string `json:"field"`
	File       string `json:"file"`
	Line       int    `json:"line"`
}

// DynamicRef records a collection call using a variable that could not be resolved.
type DynamicRef struct {
	Variable string `json:"variable"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// ScanResult holds all collection references found in a repository.
type ScanResult struct {
	RepoPath     string          `json:"repoPath"`
	Refs         []CollectionRef `json:"refs"`
	FieldRefs    []FieldRef      `json:"fieldRefs,omitempty"`
	DynamicRefs  []DynamicRef    `json:"dynamicRefs,omitempty"`
	Collections  []string        `json:"collections"` // deduplicated collection names
	FilesScanned int             `json:"filesScanned"`
	FilesSkipped int             `json:"filesSkipped,omitempty"`
}
