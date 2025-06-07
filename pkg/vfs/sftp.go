package vfs

import (
	"context"
	"errors"
	"github.com/IljaN/opencloud-sftp/pkg/vfs/spacelookup"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/pkg/sftp"
	"github.com/rs/zerolog"
	"io"

	iofs "io/fs"
	"os"
	"sync"
	"time"
)

func OpenCloudHandler(authCtx context.Context, sel *pool.Selector[gateway.GatewayAPIClient], logger zerolog.Logger) sftp.Handlers {
	root := &root{
		authCtx:    authCtx,
		gwSelector: sel,
		log:        logger,
	}

	root.log.Debug().Msg("Initializing sftp vfs")
	return sftp.Handlers{root, root, root, root}
}

type root struct {
	mu         sync.Mutex
	authCtx    context.Context
	gwSelector *pool.Selector[gateway.GatewayAPIClient]
	log        zerolog.Logger
}

func (fs *root) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	flags := r.Pflags()
	if !flags.Read {
		// sanity check
		return nil, os.ErrInvalid
	}

	return fs.OpenFile(r)
}

func (fs *root) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	flags := r.Pflags()
	if !flags.Write {
		// sanity check
		return nil, os.ErrInvalid
	}

	return fs.OpenFile(r)
}

func (fs *root) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.log.Debug().
		Str("path", r.Filepath).
		Uint32("flags", r.Flags).
		Msg("OpenFile called")

	storageSpaces, err := fs.listStorageSpaces()
	if err != nil {
		return nil, err
	}

	spc, relPath, err := spacelookup.FindSpaceForPath(r.Filepath, storageSpaces)
	if err != nil {
		return nil, err
	}

	if spc == nil {
		return nil, os.ErrNotExist
	}

	ref, err := spacelookup.MakeStorageSpaceReference(spc.Id.GetOpaqueId(), relPath)
	if err != nil {
		fs.log.Debug().Err(err).Msg("makeStorageSpaceReference error in OpenFile")
		return nil, err
	}

	// Create file if it doesn't exist and flags indicate creation
	flags := r.Pflags()
	if flags.Write && (flags.Creat || flags.Trunc) {
		client, err := fs.gwSelector.Next()
		if err != nil {
			return nil, err
		}

		// Touch the file to ensure it exists
		_, err = client.TouchFile(fs.authCtx, &storageProvider.TouchFileRequest{
			Ref: &storageProvider.Reference{ResourceId: ref.GetResourceId(), Path: relPath},
		})
		if err != nil {
			fs.log.Debug().Err(err).Msg("TouchFile error in OpenFile")
			// Ignore error - file might already exist
		}
	}

	// Return the file handler that implements WriterAt and ReaderAt
	return newSftpFileHandler(fs, &ref, r.Filepath, r.Flags), nil
}

func (fs *root) Filecmd(r *sftp.Request) error {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	switch r.Method {
	case "Setstat":
		return errors.New("setstat not supported")
	case "Rename":
		// SFTP-v2: "It is an error if there already exists a file with the name specified by newpath."
		// This varies from the POSIX specification, which allows limited replacement of target files.
		return fs.rename(r.Filepath, r.Target, false)
	case "Rmdir":
		return fs.rmdir(r.Filepath)
	case "Remove":
		// IEEE 1003.1 remove explicitly can unlink files and remove empty directories.
		// We use instead here the semantics of unlink, which is allowed to be restricted against directories.
		return fs.remove(r.Filepath)
	case "Mkdir":
		return fs.mkdir(r.Filepath)
	case "Link":
		return errors.New("hard links are not supported")
	case "Symlink":
		// NOTE: r.Filepath is the target, and r.Target is the linkpath.
		return errors.New("symbolic links are not supported")
	}

	return errors.New("unsupported")
}

func (fs *root) rename(oldpath, newpath string, allowOverwrite bool) error {
	return fs.renameFile(oldpath, newpath, allowOverwrite)
}

func (fs *root) PosixRename(r *sftp.Request) error {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	// POSIX rename allows overwriting existing files
	return fs.rename(r.Filepath, r.Target, true)
}

func (fs *root) StatVFS(r *sftp.Request) (*sftp.StatVFS, error) {
	return nil, errors.New("StatVFS not supported")
}

type listerat []os.FileInfo

// Modeled after strings.Reader's ReadAt() implementation
func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	var n int
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n = copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}

type fileInfo struct {
	name  string
	size  int64
	mode  iofs.FileMode
	mtime time.Time
	isDir bool
	sys   any
}

func (f fileInfo) Name() string {
	return f.name
}

func (f fileInfo) Size() int64 {
	return f.size
}

func (f fileInfo) Mode() iofs.FileMode {
	return f.mode
}

func (f fileInfo) ModTime() time.Time {
	return f.mtime
}

func (f fileInfo) IsDir() bool {
	return f.isDir
}

func (f fileInfo) Sys() any {
	return f.sys
}

func (fs *root) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.log.Debug().
		Str("method", r.Method).
		Str("file-path", r.Filepath).
		Msg("Filelist called")

	switch r.Method {
	case "List":
		list, err := fs.list(r.Filepath)
		return listerat(list), err
	case "Stat":
		fi, err := fs.stat(r.Filepath)
		return listerat{fi}, err
	}

	return nil, errors.New("unsupported")
}
