package spacelookup

import (
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/storagespace"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
	"strings"
)

// MakeStorageSpaceReference find a space by id and returns a relative reference
func MakeStorageSpaceReference(spaceID string, relativePath string) (storageProvider.Reference, error) {
	resourceID, err := storagespace.ParseID(spaceID)
	if err != nil {
		return storageProvider.Reference{}, err
	}
	// be tolerant about missing sharesstorageprovider id
	if resourceID.StorageId == "" && resourceID.SpaceId == utils.ShareStorageSpaceID {
		resourceID.StorageId = utils.ShareStorageProviderID
	}
	return storageProvider.Reference{
		ResourceId: &resourceID,
		Path:       utils.MakeRelativePath(relativePath),
	}, nil
}

// FindSpaceForPath takes an absolute path and a list of storage spaces, and returns the first space that matches the name in the path and the relative path within that space
func FindSpaceForPath(path string, spaces []*storageProvider.StorageSpace) (space *storageProvider.StorageSpace, relPath string, err error) {
	spaceName, relPath := SplitAbsolutePath(path)

	for k := range spaces {
		if spaces[k].GetName() == spaceName {
			space = spaces[k]
			return
		}
	}

	return nil, "", nil
}

// SplitAbsolutePath splits an absolute path into the first part which should be a space name, and a second part which is the rest of the path
// relative to that space.
func SplitAbsolutePath(path string) (string, string) {
	// Remove leading slash, if any
	trimmed := strings.TrimPrefix(path, "/")

	// Split into two parts
	parts := strings.SplitN(trimmed, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}

	first := parts[0]
	var rest string
	if len(parts) == 2 {
		rest = "/" + parts[1]
	}

	if rest == "" {
		rest = "/"
	}

	return first, rest
}
