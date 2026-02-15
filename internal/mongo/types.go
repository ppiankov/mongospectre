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
	Name        string      `json:"name"`
	Database    string      `json:"database"`
	Type        string      `json:"type"` // "collection" or "view"
	DocCount    int64       `json:"docCount"`
	Size        int64       `json:"size"`        // uncompressed data size in bytes
	AvgObjSize  int64       `json:"avgObjSize"`  // average document size in bytes
	StorageSize int64       `json:"storageSize"` // allocated storage in bytes
	Indexes     []IndexInfo `json:"indexes"`
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
	TTL    *int32      `json:"ttl,omitempty"` // TTL seconds, nil if not a TTL index
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
