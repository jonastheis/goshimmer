package ledgerstate

import (
	"time"

	"github.com/iotaledger/hive.go/objectstorage"
)

const (
	// PrefixBranchStorage defines the storage prefix for the Branch object storage.
	PrefixBranchStorage byte = iota
)

// objectStorageOptions contains a list of default settings for the object storage.
var objectStorageOptions = []objectstorage.Option{
	objectstorage.CacheTime(60 * time.Second),
}
