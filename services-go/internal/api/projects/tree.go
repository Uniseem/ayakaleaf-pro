package projects

import "go.mongodb.org/mongo-driver/v2/bson"

// The tree.
//
// A project's files are stored as folders nested inside the project document,
// and almost everything that is not "draw the file tree" wants them flat and
// wants each one's path: a compile sends paths to the compiler, an editor
// opens a path, an upload replaces one. Walking the tree lives here once, so
// that no handler grows its own recursion and gets the path separator or the
// root folder's name subtly wrong.

// EntryKind says which of the three things in a tree an entry is.
type EntryKind string

const (
	EntryFolder EntryKind = "folder"
	EntryDoc    EntryKind = "doc"
	EntryFile   EntryKind = "file"
)

// Entry is one thing in a project's tree.
type Entry struct {
	ID   bson.ObjectID `json:"id"`
	Name string        `json:"name"`
	// Path is where the entry is in the project, with '/' between folders and
	// no leading slash. The root folder itself is "", and a document directly
	// inside it is just its name -- which is the form a compile needs, because
	// that is where the compiler will write the file.
	Path   EntryPath     `json:"path"`
	Kind   EntryKind     `json:"kind"`
	Parent bson.ObjectID `json:"parentId,omitempty"`
	// Hash and Size are set for binary files, and are what tells one version
	// of an uploaded file from another.
	Hash string `json:"hash,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// EntryPath is a path inside a project.
type EntryPath = string

// Walk visits every entry in the project, depth first, folders before what is
// inside them.
//
// The root folder is not visited: it has no name anybody chose and no path, and
// every caller would have to skip it.
func (p *Project) Walk(visit func(Entry)) {
	for _, root := range p.RootFolder {
		walkFolder(root, "", visit)
	}
}

func walkFolder(folder Folder, prefix string, visit func(Entry)) {
	for _, doc := range folder.Docs {
		visit(Entry{
			ID: doc.ID, Name: doc.Name, Path: joinPath(prefix, doc.Name),
			Kind: EntryDoc, Parent: folder.ID,
		})
	}
	for _, file := range folder.FileRefs {
		visit(Entry{
			ID: file.ID, Name: file.Name, Path: joinPath(prefix, file.Name),
			Kind: EntryFile, Parent: folder.ID, Hash: file.Hash, Size: file.Size,
		})
	}
	for _, child := range folder.Folders {
		path := joinPath(prefix, child.Name)
		visit(Entry{
			ID: child.ID, Name: child.Name, Path: path,
			Kind: EntryFolder, Parent: folder.ID,
		})
		walkFolder(child, path, visit)
	}
}

// Find looks an entry up by id.
//
// Handlers use it to answer "is this document in this project", which is the
// check that stops an id from one project being used against another.
func (p *Project) Find(id bson.ObjectID) (Entry, bool) {
	var found Entry
	var ok bool
	p.Walk(func(entry Entry) {
		if !ok && entry.ID == id {
			found, ok = entry, true
		}
	})
	return found, ok
}

// FindPath looks an entry up by where it is.
func (p *Project) FindPath(path string) (Entry, bool) {
	var found Entry
	var ok bool
	p.Walk(func(entry Entry) {
		if !ok && entry.Path == path {
			found, ok = entry, true
		}
	})
	return found, ok
}

// Entries is the whole tree, flat.
func (p *Project) Entries() []Entry {
	entries := []Entry{}
	p.Walk(func(entry Entry) { entries = append(entries, entry) })
	return entries
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}
