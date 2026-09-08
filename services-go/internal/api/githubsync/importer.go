package githubsync

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Importing a repository as a new project.
//
// GitHub will hand over a whole repository as an archive in one request, which
// is far less work than walking the tree and fetching every file. The archive
// is written to disk rather than held in memory: a repository is somebody
// else's idea of how big a thing can be.

const (
	// maxArchive is the largest repository this will take.
	maxArchive = 512 << 20
	// maxImportedFile is the largest single file in one.
	maxImportedFile = 100 << 20
)

// Import makes a project out of a repository and links the two.
func (s *Service) Import(
	ctx context.Context,
	userID bson.ObjectID,
	name, repoFullName, branch string,
) (*projects.Project, error) {
	token, err := s.store.Token(ctx, userID)
	if err != nil {
		return nil, err
	}
	head, err := s.api.BranchHead(ctx, token, repoFullName, branch)
	if err != nil {
		return nil, err
	}

	archive, err := s.download(ctx, token, repoFullName, head)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = archive.Close()
		_ = os.Remove(archive.Name())
	}()

	size, err := archive.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(archive, size)
	if err != nil {
		return nil, fmt.Errorf("github sent an archive this could not read: %w", err)
	}

	project, err := s.projects.Create(ctx, userID, name, "")
	if err != nil {
		return nil, err
	}
	// A project made this way is given a history and nothing else: its files
	// are the repository's, so the usual first file would only be in the way.
	if err := s.documents.PrepareNewProject(ctx, project); err != nil {
		_ = s.projects.Delete(ctx, project.ID)
		return nil, err
	}

	if err := s.unpack(ctx, reader, project, userID); err != nil {
		// A half-imported project is worse than none: it looks like the
		// repository and is not it.
		_ = s.projects.Delete(ctx, project.ID)
		return nil, err
	}

	version, err := s.latestVersion(ctx, project.ID)
	if err != nil {
		// The files are there and the project is usable; only the link is
		// missing, and asking for it again is a button.
		if s.log != nil {
			s.log.Warn("an imported project could not be linked to its repository",
				"projectId", project.ID.Hex())
		}
		return project, nil
	}
	err = s.store.SaveState(ctx, &State{
		ProjectID:         project.ID,
		RepoFullName:      repoFullName,
		DefaultBranchName: branch,
		MergeStatus:       StatusClean,
		LastSyncCommit:    head,
		LastSyncVersion:   version,
	})
	if err != nil {
		return nil, err
	}
	return project, nil
}

// download writes the archive to a file and hands back the open file.
func (s *Service) download(ctx context.Context, token, repo, ref string) (*os.File, error) {
	body, err := s.api.Zipball(ctx, token, repo, ref)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	file, err := os.CreateTemp("", "github-import-*.zip")
	if err != nil {
		return nil, err
	}
	written, err := io.Copy(file, io.LimitReader(body, maxArchive+1))
	if err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	if written > maxArchive {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, errors.New("that repository is too large to import")
	}
	return file, nil
}

// unpack writes the files out of an archive into a project.
func (s *Service) unpack(
	ctx context.Context,
	reader *zip.Reader,
	project *projects.Project,
	userID bson.ObjectID,
) error {
	// GitHub wraps everything in one directory named after the commit. It is
	// not part of the project, and every path is under it.
	root := ""
	for _, entry := range reader.File {
		if slash := strings.IndexByte(entry.Name, '/'); slash >= 0 {
			root = entry.Name[:slash+1]
			break
		}
	}

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		path := strings.TrimPrefix(entry.Name, root)
		if !usableName(path) {
			// Anything a project cannot hold is left out rather than being
			// made to fit under a name nobody chose.
			continue
		}
		if entry.UncompressedSize64 > maxImportedFile {
			continue
		}

		file, err := entry.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(io.LimitReader(file, maxImportedFile))
		_ = file.Close()
		if err != nil {
			return err
		}

		if documents.LooksLikeText(path, content) {
			lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
			project, err = s.documents.UpsertDoc(ctx, project, path, lines, userID, "github")
		} else {
			project, err = s.documents.UpsertFile(ctx, project, path, content, userID, "github")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Export makes a repository out of a project and links the two.
func (s *Service) Export(
	ctx context.Context,
	userID, projectID bson.ObjectID,
	options RepoOptions,
) (*State, error) {
	unlock := s.lock(projectID)
	defer unlock()

	existing, err := s.store.State(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("this project is already linked to %s", existing.RepoFullName)
	}
	token, err := s.store.Token(ctx, userID)
	if err != nil {
		return nil, err
	}

	repo, err := s.api.CreateRepo(ctx, token, options)
	if err != nil {
		return nil, err
	}

	version, err := s.latestVersion(ctx, projectID)
	if err != nil {
		return nil, err
	}
	paths, err := s.history.PathsAtVersion(ctx, projectID.Hex(), version)
	if err != nil {
		return nil, err
	}

	entries := make([]TreeEntry, 0, len(paths))
	for _, path := range paths {
		content, err := s.history.FileAtVersion(ctx, projectID.Hex(), version, path)
		if err != nil {
			return nil, err
		}
		sha, err := s.api.UploadBlob(ctx, token, repo.FullName, content)
		if err != nil {
			return nil, err
		}
		entries = append(entries, TreeEntry{Path: path, SHA: sha})
	}

	// No base tree and no parent: this is the project as it stands, and the
	// repository was made a moment ago for it. The branch is moved with force
	// because what it points at is the commit GitHub made when it created the
	// repository, which nobody wants.
	tree, err := s.api.CreateTree(ctx, token, repo.FullName, entries, "")
	if err != nil {
		return nil, err
	}
	commit, err := s.api.CreateCommit(ctx, token, repo.FullName, tree,
		"The project as it was when it was exported", nil)
	if err != nil {
		return nil, err
	}
	if err := s.api.UpdateBranch(ctx, token, repo.FullName,
		repo.DefaultBranchName, commit, true); err != nil {
		return nil, err
	}

	state := &State{
		ProjectID:         projectID,
		RepoFullName:      repo.FullName,
		DefaultBranchName: repo.DefaultBranchName,
		MergeStatus:       StatusClean,
		LastSyncCommit:    commit,
		LastSyncVersion:   version,
	}
	if err := s.store.SaveState(ctx, state); err != nil {
		return nil, err
	}
	return state, nil
}
