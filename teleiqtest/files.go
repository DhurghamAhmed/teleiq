package teleiqtest

import (
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// storedFile is a file that the server holds.
type storedFile struct {
	id, unique, name, path string
	data                   []byte
}

func (f *storedFile) fileName() *string {
	if f.name == "" {
		return nil
	}
	return &f.name
}

// AddFile stores data as the file with the given file_id, as if a user had sent it.
func (s *Server) AddFile(fileID string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keep(&storedFile{id: fileID, unique: "unique-" + fileID, path: "files/" + fileID, data: data})
}

// keep stores f; s.mu must be held.
func (s *Server) keep(f *storedFile) *storedFile {
	s.files[f.id] = f
	s.paths[f.path] = f
	return f
}

// fileFor returns the file that parameter param of r sends; s.mu must be held.
func (s *Server) fileFor(r Request, param string) *storedFile {
	ref := stringOf(r.Params[param])
	name := param
	if field, ok := strings.CutPrefix(ref, "attach://"); ok {
		name = field
	}
	if up, ok := r.Uploads[name]; ok {
		s.lastFile++
		id := "file-" + strconv.FormatInt(s.lastFile, 10)
		return s.keep(&storedFile{id: id, unique: "unique-" + id, name: up.Name, path: "files/" + id + path.Ext(up.Name), data: up.Data})
	}
	if f, ok := s.files[ref]; ok {
		return f
	}
	if ref != "" && !strings.HasPrefix(ref, "http://") && !strings.HasPrefix(ref, "https://") {
		return &storedFile{id: ref, unique: "unique-" + ref}
	}
	s.lastFile++
	id := "file-" + strconv.FormatInt(s.lastFile, 10)
	return &storedFile{id: id, unique: "unique-" + id}
}

// fileInfo answers getFile; s.mu must be held.
func (s *Server) fileInfo(fileID string) (any, *teleiq.Error) {
	f, ok := s.files[fileID]
	if !ok {
		return nil, &teleiq.Error{ErrorCode: http.StatusBadRequest, Description: "Bad Request: invalid file_id"}
	}
	return s.describe(f), nil
}

func (s *Server) describe(f *storedFile) models.File {
	out := models.File{FileID: f.id, FileUniqueID: f.unique, FileSize: ptr(int64(len(f.data)))}
	if f.path != "" {
		out.FilePath = ptr(f.path)
	}
	return out
}

// download serves a file at /file/bot<token>/<file_path>, as Telegram does.
func (s *Server) download(w http.ResponseWriter, rest string) {
	_, filePath, _ := strings.Cut(rest, "/")
	s.mu.Lock()
	f, ok := s.paths[filePath]
	s.mu.Unlock()
	if !ok {
		writeError(w, &teleiq.Error{ErrorCode: http.StatusNotFound, Description: "Not Found"})
		return
	}
	_, _ = w.Write(f.data)
}
