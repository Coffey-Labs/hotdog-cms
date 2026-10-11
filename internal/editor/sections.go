package editor

import (
	"net/http"
	"sort"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
)

// apiSections is the section library for a branch, with what its fields can
// point at: the files in data/ and the collections.
func (s *Server) apiSections(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, _, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil || st == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = st.Branch
	}
	loaded, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if siteDir == "" {
		apiError(w, http.StatusBadGateway, "Couldn't fetch the site: "+trimErr(err))
		return
	}
	lib, err := build.SectionLibrary(siteDir)
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't read the sections: "+trimErr(err))
		return
	}
	data := []string{}
	cols := []string{}
	if loaded != nil {
		for name := range loaded.Data {
			data = append(data, name)
		}
		for name := range loaded.Collections {
			cols = append(cols, name)
		}
	}
	sort.Strings(data)
	sort.Strings(cols)
	writeJSON(w, http.StatusOK, map[string]any{"sections": lib, "data": data, "collections": cols, "fieldTypes": build.FieldTypes})
}
