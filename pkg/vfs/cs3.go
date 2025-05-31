package vfs

import (
	"errors"
	"fmt"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/storagespace"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"os"
	"strings"
	"syscall"
	"time"
)

func (fs *root) mkdir(dirPath string) error {
	storageSpaces, err := fs.listStorageSpaces()
	if err != nil {
		return err
	}

	spc, relPath, err := fs.findSpaceForPath(dirPath, storageSpaces)
	if err != nil {
		return err
	}

	if spc == nil {
		return os.ErrNotExist
	}

	ref, err := makeStorageSpaceReference(spc.Id.GetOpaqueId(), relPath)
	if err != nil {
		fs.log.Debug().Err(err).Msg("makeStorageSpaceReference error in Mkdir")
		return err
	}

	client, err := fs.gwSelector.Next()
	if err != nil {
		return err
	}

	mkCntRes, err := client.CreateContainer(fs.authCtx, &storageProvider.CreateContainerRequest{
		Ref: &storageProvider.Reference{ResourceId: ref.GetResourceId(), Path: relPath},
	})

	if err != nil {
		return err
	}

	respCode := mkCntRes.GetStatus().GetCode()

	if respCode != rpc.Code_CODE_OK {
		if respCode == rpc.Code_CODE_ALREADY_EXISTS {
			return fmt.Errorf("failed to create container: %w", os.ErrExist)
		}

		err = fmt.Errorf("failed to create container: %s", mkCntRes.GetStatus().GetMessage())
		fs.log.Debug().
			Err(err).
			Str("path", relPath).
			Str("code", respCode.String()).
			Msgf("Could not create container: %s", err)

		return err
	}

	return nil
}

func (fs *root) renameFile(oldpath, newpath string, allowOverwrite bool) error {
	// List storage spaces
	storageSpaces, err := fs.listStorageSpaces()
	if err != nil {
		return err
	}

	// Find space and relative path for source
	sourceSpc, sourceRelPath, err := fs.findSpaceForPath(oldpath, storageSpaces)
	if err != nil {
		return err
	}
	if sourceSpc == nil {
		return os.ErrNotExist
	}

	// Find space and relative path for target
	targetSpc, targetRelPath, err := fs.findSpaceForPath(newpath, storageSpaces)
	if err != nil {
		return err
	}
	if targetSpc == nil {
		return os.ErrNotExist
	}

	// Check if source and target are in the same storage space
	if sourceSpc.Id.GetOpaqueId() != targetSpc.Id.GetOpaqueId() {
		// Cross-space moves are not supported in this implementation
		return fmt.Errorf("cross-space moves are not supported")
	}

	// Create source reference
	sourceRef, err := makeStorageSpaceReference(sourceSpc.Id.GetOpaqueId(), sourceRelPath)
	if err != nil {
		fs.log.Debug().Err(err).Msg("makeStorageSpaceReference error for source in rename")
		return err
	}

	// Create target reference
	targetRef, err := makeStorageSpaceReference(targetSpc.Id.GetOpaqueId(), targetRelPath)
	if err != nil {
		fs.log.Debug().Err(err).Msg("makeStorageSpaceReference error for target in rename")
		return err
	}

	client, err := fs.gwSelector.Next()
	if err != nil {
		return err
	}

	// For SFTP rename (not POSIX), we need to check if target exists
	if !allowOverwrite {
		// Check if target already exists
		statResp, err := client.Stat(fs.authCtx, &storageProvider.StatRequest{
			Ref: &targetRef,
		})
		if err == nil && statResp.GetStatus().GetCode() == rpc.Code_CODE_OK {
			// Target exists, which is an error for SFTP rename
			return os.ErrExist
		}
		// If stat returned not found, that's what we want - continue with rename
	}

	// Perform the move/rename operation
	moveResp, err := client.Move(fs.authCtx, &storageProvider.MoveRequest{
		Source:      &sourceRef,
		Destination: &targetRef,
	})
	if err != nil {
		return err
	}

	if moveResp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		switch moveResp.GetStatus().GetCode() {
		case rpc.Code_CODE_NOT_FOUND:
			return os.ErrNotExist
		case rpc.Code_CODE_ALREADY_EXISTS:
			return os.ErrExist
		case rpc.Code_CODE_PERMISSION_DENIED:
			return os.ErrPermission
		default:
			err := fmt.Errorf("move failed: %s", moveResp.GetStatus().GetMessage())
			fs.log.Debug().
				Err(err).
				Str("sourcePath", oldpath).
				Str("targetPath", newpath).
				Str("code", moveResp.GetStatus().GetCode().String()).
				Msg("Could not move/rename file")
			return err
		}
	}

	return nil
}

func (fs *root) remove(pathname string) error {
	storageSpaces, err := fs.listStorageSpaces()
	if err != nil {
		return err
	}

	spc, relPath, err := fs.findSpaceForPath(pathname, storageSpaces)
	if err != nil {
		return err
	}

	if spc == nil {
		return os.ErrNotExist
	}

	// First stat the file to check if it's a directory
	ref, err := makeStorageSpaceReference(spc.Id.GetOpaqueId(), relPath)
	if err != nil {
		fs.log.Debug().Err(err).Msg("makeStorageSpaceReference error in remove")
		return err
	}

	client, err := fs.gwSelector.Next()
	if err != nil {
		return err
	}

	statResp, err := client.Stat(fs.authCtx, &storageProvider.StatRequest{
		Ref: &ref,
	})
	if err != nil {
		return err
	}

	if statResp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		if statResp.GetStatus().GetCode() == rpc.Code_CODE_NOT_FOUND {
			return os.ErrNotExist
		}
		return fmt.Errorf("stat failed: %s", statResp.GetStatus().GetMessage())
	}

	// Check if it's a directory
	if statResp.GetInfo().GetType() == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// IEEE 1003.1: implementations may opt out of allowing the unlinking of directories.
		// SFTP-v2: SSH_FXP_REMOVE may not remove directories.
		return os.ErrInvalid
	}

	// Delete the file
	deleteResp, err := client.Delete(fs.authCtx, &storageProvider.DeleteRequest{
		Ref: &ref,
	})
	if err != nil {
		return err
	}

	if deleteResp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		if deleteResp.GetStatus().GetCode() == rpc.Code_CODE_NOT_FOUND {
			return os.ErrNotExist
		}
		err = fmt.Errorf("delete failed: %s", deleteResp.GetStatus().GetMessage())
		fs.log.Debug().
			Err(err).
			Str("path", pathname).
			Str("code", deleteResp.GetStatus().GetCode().String()).
			Msg("Could not delete file")
		return err
	}

	return nil
}

func (fs *root) rmdir(pathname string) error {
	storageSpaces, err := fs.listStorageSpaces()
	if err != nil {
		return err
	}

	spc, relPath, err := fs.findSpaceForPath(pathname, storageSpaces)
	if err != nil {
		return err
	}

	if spc == nil {
		return os.ErrNotExist
	}

	ref, err := makeStorageSpaceReference(spc.Id.GetOpaqueId(), relPath)
	if err != nil {
		fs.log.Debug().Err(err).Msg("makeStorageSpaceReference error in rmdir")
		return err
	}

	client, err := fs.gwSelector.Next()
	if err != nil {
		return err
	}

	// First stat to verify it's a directory
	statResp, err := client.Stat(fs.authCtx, &storageProvider.StatRequest{
		Ref: &ref,
	})
	if err != nil {
		return err
	}

	if statResp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		if statResp.GetStatus().GetCode() == rpc.Code_CODE_NOT_FOUND {
			return os.ErrNotExist
		}
		return fmt.Errorf("stat failed: %s", statResp.GetStatus().GetMessage())
	}

	// IEEE 1003.1: If pathname is a symlink, then rmdir should fail with ENOTDIR.
	if statResp.GetInfo().GetType() != storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return syscall.ENOTDIR
	}

	// Check if directory is empty
	listResp, err := client.ListContainer(fs.authCtx, &storageProvider.ListContainerRequest{
		Ref: &ref,
	})
	if err != nil {
		return err
	}

	if listResp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return fmt.Errorf("list container failed: %s", listResp.GetStatus().GetMessage())
	}

	if len(listResp.GetInfos()) > 0 {
		return errors.New("directory not empty")
	}

	// Delete the empty directory
	deleteResp, err := client.Delete(fs.authCtx, &storageProvider.DeleteRequest{
		Ref: &ref,
	})
	if err != nil {
		return err
	}

	if deleteResp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		if deleteResp.GetStatus().GetCode() == rpc.Code_CODE_NOT_FOUND {
			return os.ErrNotExist
		}
		err = fmt.Errorf("delete directory failed: %s", deleteResp.GetStatus().GetMessage())
		fs.log.Debug().
			Err(err).
			Str("path", pathname).
			Str("code", deleteResp.GetStatus().GetCode().String()).
			Msg("Could not delete directory")
		return err
	}

	return nil
}

func (fs *root) list(dirPath string) ([]os.FileInfo, error) {

	storageSpaces, err := fs.listStorageSpaces()
	if err != nil {
		return nil, err
	}

	if dirPath == "/" {
		finfos := storageSpacesToFileInfo(storageSpaces)
		return finfos, nil
	}

	spc, relPath, err := fs.findSpaceForPath(dirPath, storageSpaces)
	if err != nil {
		return nil, err
	}

	if spc == nil {
		return nil, os.ErrNotExist
	}

	ref, err := makeStorageSpaceReference(spc.Id.GetOpaqueId(), relPath)
	if err != nil {
		fs.log.Debug().Err(err).Msg("makeStorageSpaceReference error")
		return nil, err
	}
	fs.log.Debug().
		Str("spaceId", spc.Id.GetOpaqueId()).
		Str("path", relPath).
		Msg("Created ref with space ID")

	client, err := fs.gwSelector.Next()
	if err != nil {
		return nil, err
	}

	fs.log.Debug().Msg("Calling 'ListContainer'")
	listResp, err := client.ListContainer(fs.authCtx, &storageProvider.ListContainerRequest{
		Ref: &ref,
	})

	if err != nil {
		fs.log.Debug().Err(err).Msg("ListContainer error")
		return nil, err
	}

	if listResp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		fs.log.Debug().
			Str("code", listResp.GetStatus().GetCode().String()).
			Str("message", listResp.GetStatus().GetMessage()).
			Msg("ListContainer status not OK")
		return nil, fmt.Errorf("list container failed: %s", listResp.GetStatus().GetMessage())
	}

	infos := listResp.GetInfos()
	fs.log.Debug().Int("itemCount", len(infos)).Msg("ListContainer returned items")
	fileInfos := toFileInfos(infos...)
	return fileInfos, nil

}

func (fs *root) stat(path string) (os.FileInfo, error) {
	storageSpaces, err := fs.listStorageSpaces()
	if err != nil {
		return nil, err
	}

	spc, relPath, err := fs.findSpaceForPath(path, storageSpaces)
	if err != nil {
		return nil, err
	}

	if spc == nil {
		return nil, os.ErrNotExist
	}

	client, err := fs.gwSelector.Next()
	if err != nil {
		return nil, err
	}

	ref, err := makeStorageSpaceReference(spc.Id.GetOpaqueId(), relPath)
	if err != nil {
		return nil, err
	}

	statResp, err := client.Stat(fs.authCtx, &storageProvider.StatRequest{
		Ref: &ref,
	})

	if err != nil {
		return nil, err
	}

	fi := toFileInfos(statResp.GetInfo())
	return fi[0], nil
}

func (fs *root) listStorageSpaces() ([]*storageProvider.StorageSpace, error) {
	client, err := fs.gwSelector.Next()
	if err != nil {
		return []*storageProvider.StorageSpace{}, err
	}

	lSSReq := &storageProvider.ListStorageSpacesRequest{
		FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"*"}},
	}

	lSSRes, err := client.ListStorageSpaces(fs.authCtx, lSSReq)
	if err != nil {
		return []*storageProvider.StorageSpace{}, err
	}
	if lSSRes.Status.GetCode() != rpc.Code_CODE_OK {
		return []*storageProvider.StorageSpace{}, err
	}

	return lSSRes.GetStorageSpaces(), nil
}

func splitPath(path string) (string, string) {
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

func (fs *root) findSpaceForPath(path string, spaces []*storageProvider.StorageSpace) (space *storageProvider.StorageSpace, relPath string, err error) {
	spaceName, relPath := splitPath(path)

	fs.log.Debug().
		Str("path", path).
		Str("spaceName", spaceName).
		Str("relPath", relPath).
		Msg("Resolving path to space and rel-path")

	for k := range spaces {
		if spaces[k].GetName() == spaceName {
			space = spaces[k]
			fs.log.Debug().Msgf("Found storage space %s", spaceName)
			return
		}
	}

	fs.log.Debug().Msgf("Space '%s' not found", spaceName)
	return nil, "", nil
}

func toFileInfos(rInfos ...*storageProvider.ResourceInfo) []os.FileInfo {
	var fileInfos []os.FileInfo
	for _, ri := range rInfos {
		fi := fileInfo{
			name: ri.GetName(),
			size: int64(ri.GetSize()),
			sys:  ri,
		}

		if ri.GetType() == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
			fi.mode = os.FileMode(0755) | os.ModeDir
			fi.isDir = true
		} else {
			fi.mode = os.FileMode(0644)
			fi.isDir = false
		}

		if ri.GetMtime() != nil {
			fi.mtime = time.Unix(int64(ri.GetMtime().Seconds), 0)
		}

		fileInfos = append(fileInfos, fi)
	}
	return fileInfos
}

func storageSpacesToFileInfo(spaces []*storageProvider.StorageSpace) []os.FileInfo {
	var files = []os.FileInfo{}
	for k := range spaces {
		mode := os.FileMode(0775)
		mode |= os.ModeDir
		f := fileInfo{
			name:  spaces[k].GetName(),
			mode:  mode,
			isDir: true,
			sys:   spaces[k],
		}

		if spaces[k].GetMtime() != nil {
			f.mtime = time.Unix(int64(spaces[k].GetMtime().Seconds), 0)
		}

		files = append(files, f)

	}

	return files
}

// makeStorageSpaceReference find a space by id and returns a relative reference
func makeStorageSpaceReference(spaceID string, relativePath string) (storageProvider.Reference, error) {
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
