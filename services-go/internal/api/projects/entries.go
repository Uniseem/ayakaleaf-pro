package projects

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Changing the tree.
//
// A project's files are stored inside the project document, so adding one is
// an update to a position in a nested array: "rootFolder.0.folders.2.docs".
// Those positions are worked out from the copy already in memory, which is
// what stops this from being a second traversal written slightly differently
// each time somebody needs one.
//
// Every change here bumps the project's version and returns the new one. That
// number is what the history is ordered by: a change applied without it, or
// with a number somebody else has already used, is a change the history
// records in the wrong place or not at all.

// RootFolderID is the folder everything is in.
func (p *Project) RootFolderID() (bson.ObjectID, bool) {
	if len(p.RootFolder) == 0 {
		return bson.ObjectID{}, false
	}
	return p.RootFolder[0].ID, true
}

// FolderPath is where a folder is in the stored document.
func (p *Project) FolderPath(id bson.ObjectID) (string, bool) {
	for i, root := range p.RootFolder {
		base := fmt.Sprintf("rootFolder.%d", i)
		if root.ID == id {
			return base, true
		}
		if path, ok := folderPathIn(root, base, id); ok {
			return path, true
		}
	}
	return "", false
}

func folderPathIn(folder Folder, base string, id bson.ObjectID) (string, bool) {
	for i, child := range folder.Folders {
		path := fmt.Sprintf("%s.folders.%d", base, i)
		if child.ID == id {
			return path, true
		}
		if found, ok := folderPathIn(child, path, id); ok {
			return found, true
		}
	}
	return "", false
}

// EntryPath is where one document, file or folder is in the stored document,
// along with the path of the folder holding it.
func (p *Project) EntryPath(id bson.ObjectID) (entry string, parent string, ok bool) {
	for i, root := range p.RootFolder {
		base := fmt.Sprintf("rootFolder.%d", i)
		if entry, parent, ok := entryPathIn(root, base, id); ok {
			return entry, parent, true
		}
	}
	return "", "", false
}

func entryPathIn(folder Folder, base string, id bson.ObjectID) (string, string, bool) {
	for i, doc := range folder.Docs {
		if doc.ID == id {
			return fmt.Sprintf("%s.docs.%d", base, i), base, true
		}
	}
	for i, file := range folder.FileRefs {
		if file.ID == id {
			return fmt.Sprintf("%s.fileRefs.%d", base, i), base, true
		}
	}
	for i, child := range folder.Folders {
		path := fmt.Sprintf("%s.folders.%d", base, i)
		if child.ID == id {
			return path, base, true
		}
		if entry, parent, ok := entryPathIn(child, path, id); ok {
			return entry, parent, true
		}
	}
	return "", "", false
}

// FolderByID reads a folder out of the tree.
func (p *Project) FolderByID(id bson.ObjectID) (Folder, bool) {
	for _, root := range p.RootFolder {
		if root.ID == id {
			return root, true
		}
		if folder, ok := folderIn(root, id); ok {
			return folder, true
		}
	}
	return Folder{}, false
}

func folderIn(folder Folder, id bson.ObjectID) (Folder, bool) {
	for _, child := range folder.Folders {
		if child.ID == id {
			return child, true
		}
		if found, ok := folderIn(child, id); ok {
			return found, true
		}
	}
	return Folder{}, false
}

// Taken says whether a folder already holds something by this name.
//
// Case-insensitively, because two files whose names differ only in case are
// one file on some of the systems a compile runs on, and a project that
// compiles here and not there is worse than a name that was refused.
func (f Folder) Taken(name string) bool {
	for _, doc := range f.Docs {
		if strings.EqualFold(doc.Name, name) {
			return true
		}
	}
	for _, file := range f.FileRefs {
		if strings.EqualFold(file.Name, name) {
			return true
		}
	}
	for _, folder := range f.Folders {
		if strings.EqualFold(folder.Name, name) {
			return true
		}
	}
	return false
}

// AddDoc puts a document into a folder.
func (s *Store) AddDoc(ctx context.Context, project *Project, folderID bson.ObjectID, doc DocRef) (int64, error) {
	path, ok := project.FolderPath(folderID)
	if !ok {
		return 0, ErrNotFound
	}
	update := bson.M{"$push": bson.M{path + ".docs": doc}}
	// The first document in a project becomes the one that gets compiled.
	// Somebody who has just made a project and typed into the only file in it
	// should be able to press compile without first choosing a root document.
	if project.RootDocID.IsZero() {
		update["$set"] = bson.M{"rootDoc_id": doc.ID}
	}
	return s.applyTreeChange(ctx, project.ID, update)
}

// AddFile puts a binary file into a folder.
func (s *Store) AddFile(ctx context.Context, project *Project, folderID bson.ObjectID, file FileRef) (int64, error) {
	path, ok := project.FolderPath(folderID)
	if !ok {
		return 0, ErrNotFound
	}
	return s.applyTreeChange(ctx, project.ID, bson.M{
		"$push": bson.M{path + ".fileRefs": file},
	})
}

// AddFolder puts a folder into a folder.
func (s *Store) AddFolder(ctx context.Context, project *Project, parentID bson.ObjectID, folder Folder) (int64, error) {
	path, ok := project.FolderPath(parentID)
	if !ok {
		return 0, ErrNotFound
	}
	return s.applyTreeChange(ctx, project.ID, bson.M{
		"$push": bson.M{path + ".folders": folder},
	})
}

// SetFileHash records that a file's bytes changed.
//
// The entry keeps its id, which is what makes this an edit of one file rather
// than a delete and an add: anything holding a reference to it -- a link in
// another document, a history entry -- still means this file.
func (s *Store) SetFileHash(ctx context.Context, project *Project, id bson.ObjectID, hash string, size int64) (int64, error) {
	path, _, ok := project.EntryPath(id)
	if !ok {
		return 0, ErrNotFound
	}
	return s.applyTreeChange(ctx, project.ID, bson.M{
		"$set": bson.M{
			path + ".hash":    hash,
			path + ".size":    size,
			path + ".created": time.Now().UTC(),
		},
	})
}

// RemoveEntry takes something out of the tree.
//
// What it was is not deleted: a document's text stays in docstore and a file's
// bytes stay in the blob store. This removes the only path to it, which is
// what somebody deleting a file means, and leaves the content for whatever
// restores it.
func (s *Store) RemoveEntry(ctx context.Context, project *Project, id bson.ObjectID) (int64, error) {
	_, parent, ok := project.EntryPath(id)
	if !ok {
		return 0, ErrNotFound
	}
	entry, found := project.Find(id)
	if !found {
		return 0, ErrNotFound
	}
	field := map[EntryKind]string{
		EntryDoc:    "docs",
		EntryFile:   "fileRefs",
		EntryFolder: "folders",
	}[entry.Kind]

	update := bson.M{"$pull": bson.M{parent + "." + field: bson.M{"_id": id}}}
	// A project whose root document has just been deleted has no root
	// document. Leaving the id there would make every compile fail with
	// nothing on screen to explain it.
	if project.RootDocID == id {
		update["$unset"] = bson.M{"rootDoc_id": ""}
	}
	return s.applyTreeChange(ctx, project.ID, update)
}

// RenameEntry changes what something in the tree is called.
func (s *Store) RenameEntry(ctx context.Context, project *Project, id bson.ObjectID, name string) (int64, error) {
	path, _, ok := project.EntryPath(id)
	if !ok {
		return 0, ErrNotFound
	}
	return s.applyTreeChange(ctx, project.ID, bson.M{
		"$set": bson.M{path + ".name": name},
	})
}

// SetRootDoc chooses which document a compile starts from.
func (s *Store) SetRootDoc(ctx context.Context, id, docID bson.ObjectID) error {
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"rootDoc_id": docID, "lastUpdated": time.Now().UTC()},
	})
	return err
}

// applyTreeChange writes a change to the tree and answers with the project's
// new version.
//
// The version is incremented in the same write, so two changes arriving at
// once get different numbers and the history can order them. Reading it back
// afterwards would sometimes read the other one's.
func (s *Store) applyTreeChange(ctx context.Context, id bson.ObjectID, update bson.M) (int64, error) {
	inc, ok := update["$inc"].(bson.M)
	if !ok {
		inc = bson.M{}
	}
	inc["version"] = 1
	update["$inc"] = inc

	set, ok := update["$set"].(bson.M)
	if !ok {
		set = bson.M{}
	}
	set["lastUpdated"] = time.Now().UTC()
	update["$set"] = set

	var after struct {
		Version int64 `bson:"version"`
	}
	err := s.projects.FindOneAndUpdate(ctx, bson.M{"_id": id}, update,
		options.FindOneAndUpdate().
			SetReturnDocument(options.After).
			SetProjection(bson.M{"version": 1}),
	).Decode(&after)
	if err != nil {
		return 0, err
	}
	return after.Version, nil
}

// ValidName says whether something in a project may be called this.
//
// The rules are the ones a filesystem and a compiler impose between them: a
// name becomes a real file in the directory a compile runs in, so anything
// that could leave that directory, or that no filesystem will store, is not a
// name.
// dotfiles are the hidden names that mean something to LaTeX.
//
// latexmk reads its configuration from .latexmkrc, and a project that carries
// one is relying on it: it names the engine, extra passes, and where output
// goes. An import that dropped it would produce a project that builds
// differently from the one somebody uploaded, with nothing on screen to say
// why.
var dotfiles = map[string]bool{".latexmkrc": true}

func ValidName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return fmt.Errorf("a name cannot be empty")
	case len(name) > 150:
		return fmt.Errorf("a name cannot be longer than 150 characters")
	case name == "." || name == "..":
		return fmt.Errorf("that is not a name")
	case strings.ContainsAny(name, "/\\"):
		return fmt.Errorf("a name cannot contain a slash")
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("that is not a name")
	case strings.HasPrefix(name, ".") && !dotfiles[strings.ToLower(name)]:
		// A dot at the front means "hidden" to every tool that will ever touch
		// this project outside the editor -- a checkout, a zip, a backup -- so
		// one made here is a file that quietly stops being there. The
		// exceptions are the few that LaTeX itself reads: refusing those does
		// not protect anybody, it just loses the settings of a project that
		// was imported with one.
		return fmt.Errorf("a name cannot start with a dot")
	}
	return nil
}

// MoveEntry puts something in a different folder.
//
// A pull from the old parent and a push to the new one, in one update: two
// updates would leave the entry in neither folder if the second failed, and
// the tree is the only record that it exists at all.
//
// It refuses to move a folder into itself or into anything it contains, which
// would detach that whole subtree from the root and lose it.
func (s *Store) MoveEntry(
	ctx context.Context,
	project *Project,
	id bson.ObjectID,
	folderID bson.ObjectID,
) (int64, error) {
	entry, found := project.Find(id)
	if !found {
		return 0, ErrNotFound
	}
	if entry.Parent == folderID {
		return 0, ErrNotFound
	}

	target, ok := project.FolderByID(folderID)
	if !ok {
		return 0, ErrNotFound
	}
	targetPath, ok := project.FolderPath(folderID)
	if !ok {
		return 0, ErrNotFound
	}
	_, oldParentPath, ok := project.EntryPath(id)
	if !ok {
		return 0, ErrNotFound
	}

	if entry.Kind == EntryFolder {
		// Everything under the folder being moved, including itself.
		if folderID == id {
			return 0, ErrNotFound
		}
		inside := false
		walkFolders(project, id, func(child Folder) {
			if child.ID == folderID {
				inside = true
			}
		})
		if inside {
			return 0, ErrNotFound
		}
	}

	if target.Taken(entry.Name) {
		return 0, ErrNameTaken
	}

	field := map[EntryKind]string{
		EntryDoc:    "docs",
		EntryFile:   "fileRefs",
		EntryFolder: "folders",
	}[entry.Kind]

	// The whole sub-document has to be carried across, not just the id: a
	// folder brings its contents and a file brings its hash.
	var pushed any
	switch entry.Kind {
	case EntryDoc:
		pushed = DocRef{ID: entry.ID, Name: entry.Name}
	case EntryFile:
		ref, ok := project.fileRef(id)
		if !ok {
			return 0, ErrNotFound
		}
		pushed = ref
	case EntryFolder:
		folder, ok := project.FolderByID(id)
		if !ok {
			return 0, ErrNotFound
		}
		pushed = folder
	}

	return s.applyTreeChange(ctx, project.ID, bson.M{
		"$pull": bson.M{oldParentPath + "." + field: bson.M{"_id": id}},
		"$push": bson.M{targetPath + "." + field: pushed},
	})
}

// walkFolders visits every folder inside the one named, itself included.
func walkFolders(project *Project, id bson.ObjectID, visit func(Folder)) {
	start, ok := project.FolderByID(id)
	if !ok {
		return
	}
	var descend func(Folder)
	descend = func(folder Folder) {
		visit(folder)
		for _, child := range folder.Folders {
			descend(child)
		}
	}
	descend(start)
}

// fileRef reads a binary file's whole entry, which a move has to carry over.
func (p *Project) fileRef(id bson.ObjectID) (FileRef, bool) {
	var found FileRef
	var ok bool
	var search func(Folder)
	search = func(folder Folder) {
		for _, file := range folder.FileRefs {
			if file.ID == id {
				found, ok = file, true
				return
			}
		}
		for _, child := range folder.Folders {
			search(child)
		}
	}
	for _, root := range p.RootFolder {
		search(root)
	}
	return found, ok
}

// FolderPathName is where a folder is in the project as somebody would write
// it, with '/' between folders and no leading slash. The root folder is "".
//
// Distinct from FolderPath, which answers with the path inside the stored
// document and is for building an update.
func (p *Project) FolderPathName(id bson.ObjectID) (string, bool) {
	if root, ok := p.RootFolderID(); ok && root == id {
		return "", true
	}
	entry, ok := p.Find(id)
	if !ok || entry.Kind != EntryFolder {
		return "", false
	}
	return entry.Path, true
}
