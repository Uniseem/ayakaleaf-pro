package projects

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Changing the tree.
//
// A project's files are stored inside the project document, so adding one is
// an update to a position in a nested array: "rootFolder.0.folders.2.docs".
// Those positions are worked out from the copy already in memory, which is
// what stops this from being a second traversal written slightly differently
// each time somebody needs one.

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
func (s *Store) AddDoc(ctx context.Context, project *Project, folderID bson.ObjectID, doc DocRef) error {
	path, ok := project.FolderPath(folderID)
	if !ok {
		return ErrNotFound
	}
	update := bson.M{
		"$push": bson.M{path + ".docs": doc},
		"$set":  bson.M{"lastUpdated": time.Now().UTC()},
	}
	// The first document in a project becomes the one that gets compiled.
	// Somebody who has just made a project and typed into the only file in it
	// should be able to press compile without first choosing a root document.
	if project.RootDocID.IsZero() {
		update["$set"].(bson.M)["rootDoc_id"] = doc.ID
	}
	_, err := s.projects.UpdateByID(ctx, project.ID, update)
	return err
}

// AddFolder puts a folder into a folder.
func (s *Store) AddFolder(ctx context.Context, project *Project, parentID bson.ObjectID, folder Folder) error {
	path, ok := project.FolderPath(parentID)
	if !ok {
		return ErrNotFound
	}
	_, err := s.projects.UpdateByID(ctx, project.ID, bson.M{
		"$push": bson.M{path + ".folders": folder},
		"$set":  bson.M{"lastUpdated": time.Now().UTC()},
	})
	return err
}

// RemoveEntry takes something out of the tree.
//
// What it was is not deleted: a document's text stays in docstore and a file's
// bytes stay in filestore, marked deleted there. This removes the only path to
// it, which is what somebody deleting a file means, and leaves the content for
// whatever restores it.
func (s *Store) RemoveEntry(ctx context.Context, project *Project, id bson.ObjectID) error {
	_, parent, ok := project.EntryPath(id)
	if !ok {
		return ErrNotFound
	}
	entry, found := project.Find(id)
	if !found {
		return ErrNotFound
	}
	field := map[EntryKind]string{
		EntryDoc:    "docs",
		EntryFile:   "fileRefs",
		EntryFolder: "folders",
	}[entry.Kind]

	update := bson.M{
		"$pull": bson.M{parent + "." + field: bson.M{"_id": id}},
		"$set":  bson.M{"lastUpdated": time.Now().UTC()},
	}
	// A project whose root document has just been deleted has no root
	// document. Leaving the id there would make every compile fail with
	// nothing on screen to explain it.
	if project.RootDocID == id {
		update["$unset"] = bson.M{"rootDoc_id": ""}
	}
	_, err := s.projects.UpdateByID(ctx, project.ID, update)
	return err
}

// RenameEntry changes what something in the tree is called.
func (s *Store) RenameEntry(ctx context.Context, project *Project, id bson.ObjectID, name string) error {
	path, _, ok := project.EntryPath(id)
	if !ok {
		return ErrNotFound
	}
	_, err := s.projects.UpdateByID(ctx, project.ID, bson.M{
		"$set": bson.M{path + ".name": name, "lastUpdated": time.Now().UTC()},
	})
	return err
}

// SetRootDoc chooses which document a compile starts from.
func (s *Store) SetRootDoc(ctx context.Context, id, docID bson.ObjectID) error {
	_, err := s.projects.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"rootDoc_id": docID, "lastUpdated": time.Now().UTC()},
	})
	return err
}

// ValidName says whether something in a project may be called this.
//
// The rules are the ones a filesystem and a compiler impose between them: a
// name becomes a real file in the directory a compile runs in, so anything
// that could leave that directory, or that no filesystem will store, is not a
// name.
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
	case strings.HasPrefix(name, "."):
		// Hidden files are not shown by anything here, so one would be a file
		// somebody could create and then never find again.
		return fmt.Errorf("a name cannot start with a dot")
	}
	return nil
}
