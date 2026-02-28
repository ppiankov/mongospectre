package mongo

import "time"

// Config holds MongoDB connection settings.
type Config struct {
	URI      string
	Database string // empty = all non-system databases
}

// DatabaseInfo describes a MongoDB database.
type DatabaseInfo struct {
	Name       string `json:"name"`
	SizeOnDisk int64  `json:"sizeOnDisk"`
	Empty      bool   `json:"empty"`
}

// CollectionInfo describes a MongoDB collection with stats.
type CollectionInfo struct {
	Name           string         `json:"name"`
	Database       string         `json:"database"`
	Type           string         `json:"type"` // "collection" or "view"
	DocCount       int64          `json:"docCount"`
	Size           int64          `json:"size"`           // uncompressed data size in bytes
	AvgObjSize     int64          `json:"avgObjSize"`     // average document size in bytes
	StorageSize    int64          `json:"storageSize"`    // allocated storage in bytes
	TotalIndexSize int64          `json:"totalIndexSize"` // total size of all indexes in bytes
	Indexes        []IndexInfo    `json:"indexes"`
	Validator      *ValidatorInfo `json:"validator,omitempty"`
}

// ValidatorInfo describes collection-level JSON Schema validation settings.
type ValidatorInfo struct {
	Collection       string          `json:"collection"`
	Database         string          `json:"database"`
	Schema           ValidatorSchema `json:"schema"`
	ValidationLevel  string          `json:"validationLevel,omitempty"`
	ValidationAction string          `json:"validationAction,omitempty"`
}

// ValidatorSchema is a normalized subset of MongoDB JSON Schema we analyze.
type ValidatorSchema struct {
	Required             []string                  `json:"required,omitempty"`
	AdditionalProperties *bool                     `json:"additionalProperties,omitempty"`
	Properties           map[string]ValidatorField `json:"properties,omitempty"`
}

// ValidatorField captures expected BSON types for a single schema property.
type ValidatorField struct {
	BSONTypes []string `json:"bsonTypes,omitempty"`
}

// KeyField is an ordered index key element.
type KeyField struct {
	Field     string `json:"field"`
	Direction int    `json:"direction"` // 1 (asc) or -1 (desc)
}

// IndexInfo describes a single index on a collection.
type IndexInfo struct {
	Name   string      `json:"name"`
	Key    []KeyField  `json:"key"`
	Unique bool        `json:"unique,omitempty"`
	Sparse bool        `json:"sparse,omitempty"`
	TTL    *int32      `json:"ttl,omitempty"`  // TTL seconds, nil if not a TTL index
	Size   int64       `json:"size,omitempty"` // index size in bytes from collStats.indexSizes
	Stats  *IndexStats `json:"stats,omitempty"`
}

// IndexStats holds usage statistics for an index.
type IndexStats struct {
	Ops   int64     `json:"ops"`   // number of operations that used this index
	Since time.Time `json:"since"` // when stats tracking started
}

// ServerInfo holds basic server metadata.
type ServerInfo struct {
	Version string `json:"version"`
}

// ProfileEntry represents a normalized slow-query profiler document shape.
type ProfileEntry struct {
	Database         string    `json:"database"`
	Collection       string    `json:"collection"`
	FilterFields     []string  `json:"filterFields,omitempty"`
	SortFields       []string  `json:"sortFields,omitempty"`
	ProjectionFields []string  `json:"projectionFields,omitempty"`
	DurationMillis   int64     `json:"durationMillis"`
	Timestamp        time.Time `json:"timestamp"`
	PlanSummary      string    `json:"planSummary,omitempty"`
}

// UserRole describes a single role assigned to a user.
type UserRole struct {
	Role string `json:"role" bson:"role"`
	DB   string `json:"db"   bson:"db"`
}

// UserInfo describes a MongoDB user from db.getUsers().
type UserInfo struct {
	Username string     `json:"user" bson:"user"`
	Database string     `json:"db"   bson:"db"`
	Roles    []UserRole `json:"roles" bson:"roles"`
}

// ShardingInfo captures cluster-level sharding metadata used for audit checks.
type ShardingInfo struct {
	Enabled         bool                    `json:"enabled"`
	BalancerEnabled bool                    `json:"balancerEnabled"`
	Shards          []string                `json:"shards,omitempty"`
	Collections     []ShardedCollectionInfo `json:"collections,omitempty"`
}

// ShardedCollectionInfo captures shard key and chunk metadata for one collection.
type ShardedCollectionInfo struct {
	Namespace         string           `json:"namespace"`
	Database          string           `json:"database"`
	Collection        string           `json:"collection"`
	Key               []KeyField       `json:"key"`
	ChunkCount        int64            `json:"chunkCount"`
	ChunkDistribution map[string]int64 `json:"chunkDistribution,omitempty"`
	JumboChunks       int64            `json:"jumboChunks"`
	ChunkLimitHit     bool             `json:"chunkLimitHit,omitempty"`
}

// SecurityInfo holds server security configuration for hardening audits.
type SecurityInfo struct {
	AuthEnabled          bool   `json:"authEnabled"`
	TLSMode              string `json:"tlsMode"`
	TLSAllowInvalidCerts bool   `json:"tlsAllowInvalidCerts"`
	BindIP               string `json:"bindIp"`
	AuditLogEnabled      bool   `json:"auditLogEnabled"`
	LocalhostAuthBypass  bool   `json:"localhostAuthBypass"`
}

// FieldSampleResult holds sampled field frequency data for one collection.
type FieldSampleResult struct {
	Database   string           `json:"database"`
	Collection string           `json:"collection"`
	SampleSize int64            `json:"sampleSize"`
	Fields     []FieldFrequency `json:"fields"`
}

// FieldFrequency tracks how often a field path appears and its BSON types.
type FieldFrequency struct {
	Path  string           `json:"path"`
	Count int64            `json:"count"`
	Types map[string]int64 `json:"types"`
}
