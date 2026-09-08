package githubsync

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/documents"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/history"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/session"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Syncing.
//
// A sync is a three-way merge between what the project was at the last sync,
// what it is now, and what the repository is now. None of the merging is done
// here: changes made in the project are turned into a commit, and GitHub is
// asked to merge that commit the way it merges a pull request. What is here is
// the bookkeeping around that -- which of the three has moved, what to compare
// against, and what to do when the answer is that a person has to look.

// ErrNotLinked means this project is not connected to a repository.
var ErrNotLinked = errors.New("this project is not linked to a repository")

// Settings is what an administrator decided about GitHub.
type Settings interface {
	GitHubSyncEnabled() bool
	GitHubSyncCredentials() (Credentials, bool)
}

// Service syncs projects with repositories.
type Service struct {
	log        *slog.Logger
	store      *Store
	projects   *projects.Store
	users      *users.Store
	documents  *documents.Service
	docupdater *documents.Client
	history    *history.Client
	sessions   *session.Store
	settings   Settings
	api        *api

	// Two syncs of one project at once would each work from a picture the
	// other is changing. One at a time per project, and other projects are
	// unaffected.
	locks sync.Map
}

// Options is what a Service needs.
type Options struct {
	Log        *slog.Logger
	Store      *Store
	Projects   *projects.Store
	Users      *users.Store
	Documents  *documents.Service
	Docupdater *documents.Client
	History    *history.Client
	Sessions   *session.Store
	Settings   Settings
}

// NewService builds it.
func NewService(opts Options) *Service {
	return &Service{
		log:        opts.Log,
		store:      opts.Store,
		projects:   opts.Projects,
		users:      opts.Users,
		documents:  opts.Documents,
		docupdater: opts.Docupdater,
		history:    opts.History,
		sessions:   opts.Sessions,
		settings:   opts.Settings,
		api:        newAPI(),
	}
}

// Result is what a sync ended as.
type Result struct {
	MergeStatus        string `json:"mergeStatus"`
	RepoFullName       string `json:"repoFullName"`
	UnmergedBranchName string `json:"unmergedBranchName,omitempty"`
}

// Sync brings a project and its repository together.
//
// claimResolved is somebody saying they have merged the conflict branch
// themselves. It is the only way out of the conflict state, because nothing
// here can know whether the merge on the other side was finished.
func (s *Service) Sync(
	ctx context.Context,
	userID bson.ObjectID,
	projectID bson.ObjectID,
	message string,
	claimResolved bool,
) (*Result, error) {
	unlock := s.lock(projectID)
	defer unlock()

	state, err := s.store.State(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, ErrNotLinked
	}
	token, err := s.store.Token(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Two ways of asking for the wrong thing: syncing while a conflict is
	// unresolved, and claiming to have resolved one that is not there. Both
	// are answered by saying where things stand rather than by doing anything.
	if (!claimResolved && state.MergeStatus == StatusConflict) ||
		(claimResolved && state.MergeStatus != StatusConflict) {
		return &Result{
			MergeStatus:        state.MergeStatus,
			RepoFullName:       state.RepoFullName,
			UnmergedBranchName: state.UnmergedBranchName,
		}, nil
	}

	current, err := s.latestVersion(ctx, projectID)
	if err != nil {
		return nil, err
	}
	head, err := s.api.BranchHead(ctx, token, state.RepoFullName, state.DefaultBranchName)
	if err != nil {
		return nil, err
	}

	var unmerged string
	switch state.MergeStatus {
	case StatusClean:
		unmerged, err = s.syncClean(ctx, token, userID, state, current, head, message)
	case StatusConflict:
		unmerged, err = s.syncAfterConflict(ctx, token, userID, state, current, head)
	case StatusDiverged:
		unmerged, err = s.syncDiverged(ctx, token, userID, state, current, head, message)
	default:
		return nil, fmt.Errorf("a project in an unknown sync state: %q", state.MergeStatus)
	}
	if err != nil {
		return nil, err
	}

	status := StatusClean
	if unmerged != "" {
		status = StatusConflict
	}
	return &Result{
		MergeStatus:        status,
		RepoFullName:       state.RepoFullName,
		UnmergedBranchName: unmerged,
	}, nil
}

// syncClean is the ordinary case: the two agreed last time, so what each has
// done since can be worked out by comparing against that agreement.
func (s *Service) syncClean(
	ctx context.Context,
	token string,
	userID bson.ObjectID,
	state *State,
	current int,
	head string,
	message string,
) (string, error) {
	// Nothing changed here.
	if current == state.LastSyncVersion {
		if state.LastSyncCommit == head {
			return "", nil
		}
		return "", s.pull(ctx, token, userID, state, head)
	}

	// Something may have changed here. May, because a file created and then
	// deleted is a version that changed nothing.
	ours, err := s.exportChanges(ctx, token, state, state.LastSyncVersion, current, message, state.LastSyncCommit)
	if err != nil {
		return "", err
	}
	if ours == "" {
		if head == state.LastSyncCommit {
			return "", s.store.UpdateState(ctx, state.ProjectID, bson.M{"lastSyncVersion": current})
		}
		return "", s.pull(ctx, token, userID, state, head)
	}

	if head == state.LastSyncCommit {
		// The repository has not moved, so this can go straight on the end.
		err := s.api.UpdateBranch(ctx, token, state.RepoFullName, state.DefaultBranchName, ours, false)
		if err == nil {
			return "", s.pull(ctx, token, userID, state, ours)
		}
		if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrForbidden) {
			return "", err
		}
	}

	return s.mergeAndSettle(ctx, token, userID, state, current, head, ours)
}

// syncAfterConflict is what happens once somebody says they have merged the
// branch this left for them.
func (s *Service) syncAfterConflict(
	ctx context.Context,
	token string,
	userID bson.ObjectID,
	state *State,
	current int,
	head string,
) (string, error) {
	// Nothing changed here while the conflict was open, so the repository is
	// simply the answer.
	if current == state.ConflictVersion {
		version, err := s.applyRepo(ctx, token, userID, state, head)
		if err != nil {
			return "", err
		}
		return "", s.store.UpdateState(ctx, state.ProjectID, bson.M{
			"mergeStatus":        StatusClean,
			"lastSyncVersion":    version,
			"lastSyncCommit":     head,
			"conflictVersion":    0,
			"unmergedBranchName": "",
			"unmergedBranchHead": "",
		})
	}

	// Work was done here while the conflict was open. It has to go on top of
	// what was already pushed for the conflict, not on top of the last clean
	// sync, or it would be sent twice.
	ours, err := s.exportChanges(ctx, token, state,
		state.ConflictVersion, current,
		"[Changes made in the editor while the conflict was open]",
		state.UnmergedBranchHead)
	if err != nil {
		return "", err
	}
	if ours == "" {
		version, err := s.applyRepo(ctx, token, userID, state, head)
		if err != nil {
			return "", err
		}
		return "", s.store.UpdateState(ctx, state.ProjectID, bson.M{
			"mergeStatus":        StatusClean,
			"lastSyncVersion":    version,
			"lastSyncCommit":     head,
			"conflictVersion":    0,
			"unmergedBranchName": "",
			"unmergedBranchHead": "",
		})
	}

	return s.mergeAndSettle(ctx, token, userID, state, current, head, ours)
}

// syncDiverged is the awkward case: the repository was rewritten, so the
// commit this project was anchored to is not on the branch any more and there
// is nothing to compare against on that side.
//
// What is left is to compare the three pictures file by file and rebuild the
// history GitHub needs in order to do the merge -- a commit for the shared
// past, one for what the repository did, one for what the editor did.
func (s *Service) syncDiverged(
	ctx context.Context,
	token string,
	userID bson.ObjectID,
	state *State,
	current int,
	head string,
	message string,
) (string, error) {
	if current == state.LastSyncVersion {
		// Nothing was done here, so the repository wins outright.
		version, err := s.applyRepo(ctx, token, userID, state, head)
		if err != nil {
			return "", err
		}
		return "", s.store.UpdateState(ctx, state.ProjectID, bson.M{
			"mergeStatus": StatusClean, "lastSyncVersion": version, "lastSyncCommit": head,
		})
	}

	base, err := s.history.Snapshot(ctx, state.ProjectID.Hex(), state.LastSyncVersion)
	if err != nil {
		return "", err
	}
	local, err := s.history.Snapshot(ctx, state.ProjectID.Hex(), current)
	if err != nil {
		return "", err
	}
	remote, err := s.api.Tree(ctx, token, state.RepoFullName, head)
	if err != nil {
		return "", err
	}

	plan, err := s.plan(ctx, token, state, current, base, local, remote)
	if err != nil {
		return "", err
	}

	if !plan.changed {
		return "", s.store.UpdateState(ctx, state.ProjectID, bson.M{
			"mergeStatus": StatusClean, "lastSyncVersion": current, "lastSyncCommit": head,
		})
	}

	if !plan.conflicted {
		commit := head
		if len(plan.ours) > 0 {
			commit, err = s.commitEntries(ctx, token, state.RepoFullName, head, plan.ours, message)
			if err != nil {
				return "", err
			}
			if err := s.api.UpdateBranch(ctx, token, state.RepoFullName,
				state.DefaultBranchName, commit, false); err != nil {
				return "", err
			}
		}
		version, err := s.applyRepo(ctx, token, userID, state, commit)
		if err != nil {
			return "", err
		}
		return "", s.store.UpdateState(ctx, state.ProjectID, bson.M{
			"mergeStatus": StatusClean, "lastSyncVersion": version, "lastSyncCommit": commit,
		})
	}

	// A shared past that GitHub can merge from, built out of the files that
	// are in conflict as they were at the last sync. Then one commit for each
	// side's changes to those files, and the merge is an ordinary one.
	shared, err := s.commitEntries(ctx, token, state.RepoFullName, head, plan.base,
		"[Sync: the files as they were when this project and this repository last agreed]")
	if err != nil {
		return "", err
	}
	theirs, err := s.commitEntries(ctx, token, state.RepoFullName, shared, plan.theirs,
		"[Sync: the changes made in the repository]")
	if err != nil {
		return "", err
	}
	ours, err := s.commitEntries(ctx, token, state.RepoFullName, shared,
		append(append([]TreeEntry{}, plan.ours...), plan.oursConflicting...), message)
	if err != nil {
		return "", err
	}

	if err := s.api.UpdateBranch(ctx, token, state.RepoFullName,
		state.DefaultBranchName, theirs, false); err != nil {
		return "", err
	}
	return s.mergeAndSettle(ctx, token, userID, state, current, theirs, ours)
}

// mergeAndSettle asks GitHub to merge our commit into the branch, and records
// whichever of the two answers came back.
func (s *Service) mergeAndSettle(
	ctx context.Context,
	token string,
	userID bson.ObjectID,
	state *State,
	current int,
	head string,
	ours string,
) (string, error) {
	merged, branch, conflict, err := s.mergeThroughBranch(ctx, token,
		state.RepoFullName, state.DefaultBranchName, ours)
	if err != nil {
		return "", err
	}
	if conflict {
		return branch, s.store.UpdateState(ctx, state.ProjectID, bson.M{
			"mergeStatus":        StatusConflict,
			"lastSyncCommit":     head,
			"unmergedBranchName": branch,
			"unmergedBranchHead": ours,
			"conflictVersion":    current,
		})
	}

	version, err := s.applyRepo(ctx, token, userID, state, merged)
	if err != nil {
		return "", err
	}
	return "", s.store.UpdateState(ctx, state.ProjectID, bson.M{
		"mergeStatus":        StatusClean,
		"lastSyncVersion":    version,
		"lastSyncCommit":     merged,
		"conflictVersion":    0,
		"unmergedBranchName": "",
		"unmergedBranchHead": "",
	})
}

// pull takes the repository as it is and records the agreement.
func (s *Service) pull(ctx context.Context, token string, userID bson.ObjectID, state *State, commit string) error {
	version, err := s.applyRepo(ctx, token, userID, state, commit)
	if err != nil {
		return err
	}
	return s.store.UpdateState(ctx, state.ProjectID, bson.M{
		"lastSyncVersion": version, "lastSyncCommit": commit,
	})
}

// --- moving files between the two sides ------------------------------------

// applyRepo makes the project look like the repository at a commit.
//
// Only what differs is touched: a file whose contents are already the ones in
// the repository is left alone, so a sync that changes one file does not
// rewrite the history of every other.
func (s *Service) applyRepo(
	ctx context.Context,
	token string,
	userID bson.ObjectID,
	state *State,
	commit string,
) (int, error) {
	project, _, err := s.projects.Get(ctx, state.ProjectID, userID)
	if err != nil {
		return 0, err
	}
	current, err := s.latestVersion(ctx, state.ProjectID)
	if err != nil {
		return 0, err
	}
	here, err := s.history.Snapshot(ctx, state.ProjectID.Hex(), current)
	if err != nil {
		return 0, err
	}
	there, err := s.api.Tree(ctx, token, state.RepoFullName, commit)
	if err != nil {
		return 0, err
	}

	var refused []string
	keep := map[string]bool{}
	var write []string
	for path, sha := range there {
		if !usableName(path) {
			refused = append(refused, path)
			continue
		}
		keep[path] = true
		if hashOf(here[path]) == sha {
			continue
		}
		write = append(write, path)
	}
	if len(refused) > 0 && s.log != nil {
		// Not fatal: the rest of the repository is still worth having, and a
		// file with an impossible name would have to be renamed there anyway.
		s.log.Warn("some files in the repository cannot be part of a project",
			slog.String("projectId", state.ProjectID.Hex()),
			slog.Any("files", refused))
	}

	for _, path := range write {
		content, err := s.api.ReadFile(ctx, token, state.RepoFullName, commit, path)
		if err != nil {
			return 0, err
		}
		if documents.LooksLikeText(path, content) {
			lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
			project, err = s.documents.UpsertDoc(ctx, project, path, lines, userID, "github")
		} else {
			project, err = s.documents.UpsertFile(ctx, project, path, content, userID, "github")
		}
		if err != nil {
			return 0, err
		}
	}

	for path := range here {
		if keep[path] {
			continue
		}
		project, err = s.documents.DeletePath(ctx, project, path, userID, "github")
		if err != nil {
			return 0, err
		}
	}

	return s.latestVersion(ctx, state.ProjectID)
}

// exportChanges turns what changed in the project into one commit.
//
// An empty answer means nothing changed after all -- a file made and unmade
// leaves a version behind and no difference.
func (s *Service) exportChanges(
	ctx context.Context,
	token string,
	state *State,
	from, to int,
	message string,
	parent string,
) (string, error) {
	changes, err := s.history.Diff(ctx, state.ProjectID.Hex(), from, to)
	if err != nil {
		return "", err
	}
	was, err := s.history.PathsAtVersion(ctx, state.ProjectID.Hex(), from)
	if err != nil {
		return "", err
	}
	existed := map[string]bool{}
	for _, path := range was {
		existed[path] = true
	}

	remove := map[string]bool{}
	write := map[string]bool{}
	for _, change := range changes {
		switch change.Operation {
		case "removed":
			// Only if it was there at the last sync. A file created and
			// deleted since has nothing to remove on the other side.
			if existed[change.Pathname] {
				remove[change.Pathname] = true
			}
		case "renamed":
			if existed[change.Pathname] {
				remove[change.Pathname] = true
			}
			write[change.NewPathname] = true
		case "added", "edited":
			write[change.Pathname] = true
		}
	}
	// A rename that puts a new file under the old name: the name is both
	// removed and written, and writing is what was meant.
	for path := range write {
		delete(remove, path)
	}
	if len(remove) == 0 && len(write) == 0 {
		return "", nil
	}

	entries := make([]TreeEntry, 0, len(remove)+len(write))
	for path := range remove {
		entries = append(entries, TreeEntry{Path: path})
	}
	for path := range write {
		content, err := s.history.FileAtVersion(ctx, state.ProjectID.Hex(), to, path)
		if err != nil {
			return "", err
		}
		sha, err := s.api.UploadBlob(ctx, token, state.RepoFullName, content)
		if err != nil {
			return "", err
		}
		entries = append(entries, TreeEntry{Path: path, SHA: sha})
	}
	return s.commitEntries(ctx, token, state.RepoFullName, parent, entries, message)
}

// plan works out, file by file, who changed what -- the comparison that is
// only needed when there is no shared commit to ask GitHub about.
type syncPlan struct {
	// ours are files only this side changed, which can be applied without a
	// merge.
	ours []TreeEntry
	// theirs, oursConflicting and base are the three sides of the files both
	// changed, used to build a history GitHub can merge.
	theirs          []TreeEntry
	oursConflicting []TreeEntry
	base            []TreeEntry
	conflicted      bool
	changed         bool
}

func (s *Service) plan(
	ctx context.Context,
	token string,
	state *State,
	current int,
	base, local map[string]history.File,
	remote map[string]string,
) (*syncPlan, error) {
	plan := &syncPlan{}

	paths := map[string]bool{}
	for path := range base {
		paths[path] = true
	}
	for path := range local {
		paths[path] = true
	}
	for path := range remote {
		paths[path] = true
	}

	for path := range paths {
		baseHash := hashOf(base[path])
		localHash := hashOf(local[path])
		remoteHash := remote[path]

		// Anything this side has that the repository has never seen has to be
		// uploaded before it can be named in a tree.
		if localHash != "" && localHash != baseHash && localHash != remoteHash {
			content, err := s.contentAt(ctx, state, current, path, local[path])
			if err != nil {
				return nil, err
			}
			if _, err := s.api.UploadBlob(ctx, token, state.RepoFullName, content); err != nil {
				return nil, err
			}
		}

		if localHash == remoteHash {
			continue
		}
		plan.changed = true
		if baseHash == remoteHash {
			// Only this side moved.
			plan.ours = append(plan.ours, TreeEntry{Path: path, SHA: localHash})
			continue
		}
		if baseHash == localHash {
			// Only the repository moved. Nothing to push; it arrives when the
			// repository is applied back.
			continue
		}
		plan.conflicted = true
		plan.theirs = append(plan.theirs, TreeEntry{Path: path, SHA: remoteHash})
		plan.oursConflicting = append(plan.oursConflicting, TreeEntry{Path: path, SHA: localHash})
		plan.base = append(plan.base, TreeEntry{Path: path, SHA: baseHash})
	}
	return plan, nil
}

// contentAt is the bytes of a file at a version, from the snapshot if the
// snapshot has them and from the history store otherwise.
func (s *Service) contentAt(
	ctx context.Context,
	state *State,
	version int,
	path string,
	file history.File,
) ([]byte, error) {
	if file.Content != nil {
		return []byte(*file.Content), nil
	}
	return s.history.FileAtVersion(ctx, state.ProjectID.Hex(), version, path)
}

// commitEntries writes a commit containing the given changes on top of a
// parent.
func (s *Service) commitEntries(
	ctx context.Context,
	token, repo, parent string,
	entries []TreeEntry,
	message string,
) (string, error) {
	baseTree, err := s.api.CommitTree(ctx, token, repo, parent)
	if err != nil {
		return "", err
	}
	tree, err := s.api.CreateTree(ctx, token, repo, entries, baseTree)
	if err != nil {
		return "", err
	}
	return s.api.CreateCommit(ctx, token, repo, tree, message, []string{parent})
}

// mergeThroughBranch puts our commit on a branch of its own and asks GitHub to
// merge it.
//
// A branch, because GitHub's merge is between two branches and because a
// conflict has to leave something behind for a person to open: the branch is
// what they merge by hand.
func (s *Service) mergeThroughBranch(
	ctx context.Context,
	token, repo, branch, commit string,
) (merged string, temp string, conflict bool, err error) {
	temp = branchName()
	if err := s.api.CreateBranch(ctx, token, repo, temp, commit); err != nil {
		return "", "", false, err
	}

	merged, err = s.api.Merge(ctx, token, repo, branch, temp)
	if errors.Is(err, ErrConflict) {
		return "", temp, true, nil
	}
	if err != nil {
		return "", "", false, err
	}
	// Merged, so the branch has done its job. Failing to remove it leaves
	// clutter and nothing worse.
	if err := s.api.DeleteBranch(ctx, token, repo, temp); err != nil && s.log != nil {
		s.log.Warn("could not remove the branch a sync merged through",
			slog.String("repo", repo), slog.String("branch", temp))
	}
	return merged, "", false, nil
}

// latestVersion is where the project is, with everything anybody has typed
// written into the history first.
func (s *Service) latestVersion(ctx context.Context, projectID bson.ObjectID) (int, error) {
	if err := s.docupdater.Flush(ctx, projectID); err != nil {
		return 0, err
	}
	version, err := s.history.LatestVersion(ctx, projectID.Hex())
	if err != nil {
		return 0, err
	}
	return version.Version, nil
}

func (s *Service) lock(projectID bson.ObjectID) func() {
	value, _ := s.locks.LoadOrStore(projectID.Hex(), &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// --- small things ----------------------------------------------------------

// hashOf is git's name for a file, whichever way the history holds it.
//
// The comparison at the heart of a sync is between a name GitHub gave a file
// and a name the history gave the same file, so the two have to be the same
// kind of name -- which they are, because the history uses git's.
func hashOf(file history.File) string {
	if file.Hash != "" {
		return file.Hash
	}
	if file.Content == nil {
		return ""
	}
	content := []byte(*file.Content)
	sum := sha1.New()
	fmt.Fprintf(sum, "blob %d", len(content))
	sum.Write([]byte{0})
	sum.Write(content)
	return hex.EncodeToString(sum.Sum(nil))
}

// usableName says whether a path from a repository can be a file in a project.
func usableName(path string) bool {
	if path == "" || strings.ContainsRune(path, 0) || strings.HasPrefix(path, "/") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
		if strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

// branchName is what a conflict is left on. Dated, because somebody looking at
// a repository weeks later should be able to tell which one is theirs.
func branchName() string {
	return "ayakaleaf-" + time.Now().UTC().Format("2006-01-02-1504")
}
