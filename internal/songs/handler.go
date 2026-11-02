package songs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

type Handler struct {
	svc      *Service
	arr      *ArrangementService
	tags     *TagService
	revs     *RevisionRepository
	practice *PracticeRepository
	health   *HealthService
	sugg     *SuggestionService
	trans    *Translator
	cfg      HandlerConfig
	renderer *httpx.Renderer
}

type HandlerConfig struct {
	PublicBaseURL   string
	TranslateAPIURL string
	TranslateAPIKey string
	PrimaryLanguage string
}

// ---- Song handlers ----

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{
		Query:           q.Get("q"),
		Key:             q.Get("key"),
		TagSlugs:        q["tag"],
		IncludeArchived: q.Get("archived") == "1",
		Sort:            q.Get("sort"),
	}
	items, err := h.svc.Search(r.Context(), f)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	groups, err := h.tags.ListGrouped(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	selected := map[string]bool{}
	for _, s := range f.TagSlugs {
		selected[s] = true
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_list", map[string]any{
		"Songs":            items,
		"Filter":           f,
		"TagGroups":        groups,
		"SelectedTagSlugs": selected,
	})
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, r, http.StatusOK, "songs_form", map[string]any{"Song": Song{}})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	in := parseSongInput(r)
	song, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.renderer.Render(w, r, http.StatusUnprocessableEntity, "songs_form", map[string]any{
			"Song": song, "Error": err.Error(),
		})
		return
	}
	// Straight into "create first arrangement" so the workflow is obvious.
	http.Redirect(w, r, "/songs/"+song.ID+"/arrangements/new", http.StatusSeeOther)
}

func (h *Handler) View(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	song, err := h.svc.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	arrangements, err := h.arr.List(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	history, err := h.svc.History(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	tags, err := h.tags.TagsForSong(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	type arrRow struct {
		Arrangement
		Keys []ArrangementKey
	}
	var rows []arrRow
	for _, a := range arrangements {
		keys, _ := h.arr.Keys(r.Context(), a.ID)
		rows = append(rows, arrRow{Arrangement: a, Keys: keys})
	}

	h.renderer.Render(w, r, http.StatusOK, "songs_detail", map[string]any{
		"Song":          song,
		"Arrangements":  rows,
		"History":       history,
		"Tags":          tags,
		"PracticeToken": r.URL.Query().Get("practice"),
	})
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	song, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_form", map[string]any{"Song": song})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.svc.Update(r.Context(), id, parseSongInput(r)); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+id, http.StatusSeeOther)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

func (h *Handler) AddHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := time.Parse("2006-01-02", r.FormValue("used_on"))
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	_ = h.svc.AddHistory(r.Context(), KeyHistory{
		SongID: id,
		UsedOn: t,
		Key:    strings.TrimSpace(r.FormValue("key")),
		Notes:  strings.TrimSpace(r.FormValue("notes")),
	})
	http.Redirect(w, r, "/songs/"+id, http.StatusSeeOther)
}

// ---- Arrangement handlers ----

func (h *Handler) NewArrangement(w http.ResponseWriter, r *http.Request) {
	song, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_arrangement_form", map[string]any{
		"Song":        song,
		"Arrangement": Arrangement{Name: "Original"},
		"IsNew":       true,
		"MakeDefault": true,
	})
}

func (h *Handler) CreateArrangement(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	in := parseArrangementInput(r)
	makeDefault := r.FormValue("make_default") == "1"
	a, err := h.arr.Create(r.Context(), songID, in, makeDefault)
	if err != nil {
		song, _ := h.svc.Get(r.Context(), songID)
		h.renderer.Render(w, r, http.StatusUnprocessableEntity, "songs_arrangement_form", map[string]any{
			"Song": song, "Arrangement": Arrangement{Name: in.Name},
			"Error": err.Error(), "IsNew": true,
		})
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+a.ID, http.StatusSeeOther)
}

func (h *Handler) ViewArrangement(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := h.arr.Get(r.Context(), arrID)
	if err != nil || a.SongID != songID {
		http.NotFound(w, r)
		return
	}

	displayKey := r.URL.Query().Get("key")
	offsetStr := r.URL.Query().Get("semitones")
	offset, _ := strconv.Atoi(offsetStr)
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))

	// Resolve display key
	resolvedKey := displayKey
	if resolvedKey == "" {
		if k, err := h.arr.DefaultKey(r.Context(), arrID); err == nil {
			resolvedKey = k.Key
		}
	}
	if resolvedKey == "" {
		resolvedKey = a.OriginalKey
	}
	if offset != 0 && resolvedKey != "" {
		resolvedKey = TransposeKey(resolvedKey, offset)
	}

	chart, resolvedKey, resolvedLang, translationMissing, err := h.arr.ChartForLanguage(r.Context(), arrID, resolvedKey, lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	keys, err := h.arr.Keys(r.Context(), arrID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	translations, err := h.arr.ListTranslations(r.Context(), arrID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	h.renderer.Render(w, r, http.StatusOK, "songs_arrangement_view", map[string]any{
		"Song":               song,
		"Arrangement":        a,
		"Keys":               keys,
		"DisplayKey":         resolvedKey,
		"Offset":             offset,
		"Chart":              chart,
		"RequestedLang":      lang,
		"ResolvedLang":       resolvedLang,
		"TranslationMissing": translationMissing,
		"Translations":       translations,
	})
}

func (h *Handler) EditArrangement(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := h.arr.Get(r.Context(), arrID)
	if err != nil || a.SongID != songID {
		http.NotFound(w, r)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_arrangement_form", map[string]any{
		"Song": song, "Arrangement": a, "IsNew": false,
	})
}

func (h *Handler) UpdateArrangement(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	if _, err := h.arr.Update(r.Context(), arrID, parseArrangementInput(r)); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID, http.StatusSeeOther)
}

func (h *Handler) DeleteArrangement(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	if err := h.arr.Delete(r.Context(), chi.URLParam(r, "aid")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID, http.StatusSeeOther)
}

func (h *Handler) SetDefaultArrangement(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	if err := h.arr.SetDefault(r.Context(), songID, chi.URLParam(r, "aid")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID, http.StatusSeeOther)
}

// ---- Arrangement key handlers ----

func (h *Handler) CreateArrangementKey(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	name := r.FormValue("name")
	key := r.FormValue("key")
	makeDefault := r.FormValue("make_default") == "1"
	if _, err := h.arr.CreateKey(r.Context(), arrID, name, key, makeDefault); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID, http.StatusSeeOther)
}

func (h *Handler) DeleteArrangementKey(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	if err := h.arr.DeleteKey(r.Context(), arrID, chi.URLParam(r, "kid")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID, http.StatusSeeOther)
}

func (h *Handler) SetDefaultArrangementKey(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	if err := h.arr.SetDefaultKey(r.Context(), arrID, chi.URLParam(r, "kid")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID, http.StatusSeeOther)
}

// ---- PDF ----

func (h *Handler) DownloadPDF(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	arrID := chi.URLParam(r, "aid")
	if arrID == "" {
		a, err := h.arr.Default(r.Context(), songID)
		if err != nil {
			http.Error(w, "no default arrangement", http.StatusNotFound)
			return
		}
		arrID = a.ID
	}
	displayKey := r.URL.Query().Get("key")
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, "", lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write(data)
}

// ---- Import ----

func (h *Handler) ShowImport(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, r, http.StatusOK, "songs_import", nil)
}

func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httpx.ServerError(w, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	content := string(raw)
	format := DetectFormat(header.Filename, content)
	baseName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))

	var parsed ParsedSong
	switch format {
	case "chordpro":
		parsed = ParseChordPro(content)
		parsed.Format = "chordpro"
	case "opensong":
		p, err := ParseOpenSong(content)
		if err != nil {
			h.renderer.Render(w, r, http.StatusUnprocessableEntity, "songs_import", map[string]any{"Error": "OpenSong parse error: " + err.Error()})
			return
		}
		parsed = p
	case "openlyrics":
		p, err := ParseOpenLyrics(content)
		if err != nil {
			h.renderer.Render(w, r, http.StatusUnprocessableEntity, "songs_import", map[string]any{"Error": "OpenLyrics parse error: " + err.Error()})
			return
		}
		parsed = p
	default:
		parsed = ParsedSong{Lyrics: content, Format: "text", Source: content}
	}
	if parsed.Title == "" {
		parsed.Title = baseName
	}

	song, err := h.svc.Create(r.Context(), CreateSongInput{Title: parsed.Title, Author: parsed.Author})
	if err != nil {
		h.renderer.Render(w, r, http.StatusUnprocessableEntity, "songs_import", map[string]any{"Error": err.Error()})
		return
	}
	_, err = h.arr.Create(r.Context(), song.ID, CreateArrangementInput{
		Name:          "Original",
		BPM:           parsed.Tempo,
		OriginalKey:   parsed.Key,
		SourceContent: parsed.Source,
		Format:        parsed.Format,
	}, true)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+song.ID, http.StatusSeeOther)
}

// ---- Tags (unchanged) ----

func (h *Handler) TagsIndex(w http.ResponseWriter, r *http.Request) {
	groups, err := h.tags.ListGrouped(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_tags", map[string]any{"TagGroups": groups})
}

func (h *Handler) CreateTagGroup(w http.ResponseWriter, r *http.Request) {
	if name := strings.TrimSpace(r.FormValue("name")); name != "" {
		_, _ = h.tags.CreateGroup(r.Context(), name)
	}
	http.Redirect(w, r, "/songs/tags", http.StatusSeeOther)
}

func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.FormValue("group_id"))
	name := strings.TrimSpace(r.FormValue("name"))
	if groupID != "" && name != "" {
		_, _ = h.tags.CreateTag(r.Context(), groupID, name)
	}
	http.Redirect(w, r, "/songs/tags", http.StatusSeeOther)
}

func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	_ = h.tags.DeleteTag(r.Context(), chi.URLParam(r, "id"))
	http.Redirect(w, r, "/songs/tags", http.StatusSeeOther)
}

// ---- helpers ----

func parseSongInput(r *http.Request) CreateSongInput {
	return CreateSongInput{
		Title:           strings.TrimSpace(r.FormValue("title")),
		Author:          strings.TrimSpace(r.FormValue("author")),
		YouTubeURL:      strings.TrimSpace(r.FormValue("youtube_url")),
		CCLINumber:      strings.TrimSpace(r.FormValue("ccli_number")),
		CopyrightHolder: strings.TrimSpace(r.FormValue("copyright_holder")),
		CopyrightDate:   strings.TrimSpace(r.FormValue("copyright_date")),
		Publisher:       strings.TrimSpace(r.FormValue("publisher")),
		Scripture:       strings.TrimSpace(r.FormValue("scripture")),
		Notes:           strings.TrimSpace(r.FormValue("notes")),
		Archived:        r.FormValue("archived") == "1",
	}
}

func parseArrangementInput(r *http.Request) CreateArrangementInput {
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))
	length, _ := strconv.Atoi(r.FormValue("length_seconds"))
	format := r.FormValue("format")
	if format == "" {
		format = "chordpro"
	}
	return CreateArrangementInput{
		Name:          strings.TrimSpace(r.FormValue("name")),
		BPM:           bpm,
		Meter:         strings.TrimSpace(r.FormValue("meter")),
		LengthSeconds: length,
		Sequence:      strings.TrimSpace(r.FormValue("sequence")),
		Notes:         strings.TrimSpace(r.FormValue("notes")),
		OriginalKey:   strings.TrimSpace(r.FormValue("original_key")),
		SourceContent: r.FormValue("source_content"),
		Format:        format,
		Archived:      r.FormValue("archived") == "1",
	}
}


// ---- Practice links ----

func (h *Handler) CreatePracticeLink(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrangementID := strings.TrimSpace(r.FormValue("arrangement_id"))
	displayKey := strings.TrimSpace(r.FormValue("key"))
	ttl := 30 * 24 * time.Hour
	if days, _ := strconv.Atoi(r.FormValue("ttl_days")); days > 0 {
		ttl = time.Duration(days) * 24 * time.Hour
	}

	pt, err := h.practice.Create(r.Context(), songID, arrangementID, displayKey, ttl)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"?practice="+pt.Token, http.StatusSeeOther)
}

func (h *Handler) ViewPractice(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	pt, err := h.practice.Get(r.Context(), token)
	if err != nil {
		http.Error(w, "This link has expired or has been revoked.", http.StatusGone)
		return
	}

	song, err := h.svc.Get(r.Context(), pt.SongID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	arrID := pt.ArrangementID
	if arrID == "" {
		a, err := h.arr.Default(r.Context(), pt.SongID)
		if err != nil {
			http.Error(w, "No arrangement for this song", http.StatusNotFound)
			return
		}
		arrID = a.ID
	}
	arr, err := h.arr.Get(r.Context(), arrID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	displayKey := r.URL.Query().Get("key")
	if displayKey == "" {
		displayKey = pt.DisplayKey
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("semitones"))
	if offset != 0 && displayKey != "" {
		displayKey = TransposeKey(displayKey, offset)
	}
	if displayKey == "" {
		if k, err := h.arr.DefaultKey(r.Context(), arrID); err == nil {
			displayKey = k.Key
		}
		if displayKey == "" {
			displayKey = arr.OriginalKey
		}
	}

	lang := strings.TrimSpace(r.URL.Query().Get("lang"))

	chart, displayKey, resolvedLang, translationMissing, err := h.arr.ChartForLanguage(r.Context(), arrID, displayKey, lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	translations, _ := h.arr.ListTranslations(r.Context(), arrID)

	h.renderer.Render(w, r, http.StatusOK, "songs_practice", map[string]any{
		"Token":              token,
		"Song":               song,
		"Arrangement":        arr,
		"DisplayKey":         displayKey,
		"Offset":             offset,
		"Chart":              chart,
		"ExpiresAt":          pt.ExpiresAt,
		"YouTubeID":          youtubeID(song.YouTubeURL),
		"RequestedLang":      lang,
		"ResolvedLang":       resolvedLang,
		"TranslationMissing": translationMissing,
		"Translations":       translations,
	})
}

func (h *Handler) ViewPracticePDF(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	pt, err := h.practice.Get(r.Context(), token)
	if err != nil {
		http.Error(w, "expired", http.StatusGone)
		return
	}
	song, err := h.svc.Get(r.Context(), pt.SongID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	arrID := pt.ArrangementID
	if arrID == "" {
		a, err := h.arr.Default(r.Context(), pt.SongID)
		if err != nil {
			http.Error(w, "no arrangement", http.StatusNotFound)
			return
		}
		arrID = a.ID
	}
	displayKey := r.URL.Query().Get("key")
	if displayKey == "" {
		displayKey = pt.DisplayKey
	}
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	qrURL := h.publicBaseURL(r) + "/practice/" + token

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, qrURL, lang)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write(data)
}

func (h *Handler) RevokePracticeLink(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	_ = h.practice.Revoke(r.Context(), token)
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

// ---- Revisions ----

func (h *Handler) ArrangementHistory(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	arr, err := h.arr.Get(r.Context(), arrID)
	if err != nil || arr.SongID != songID {
		http.NotFound(w, r)
		return
	}
	revs, err := h.revs.List(r.Context(), arrID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_revisions", map[string]any{
		"Song":        song,
		"Arrangement": arr,
		"Revisions":   revs,
	})
}

func (h *Handler) RestoreRevision(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	revID := chi.URLParam(r, "rid")
	if _, err := h.arr.RestoreRevision(r.Context(), arrID, revID); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID+"/history", http.StatusSeeOther)
}

// ---- Health ----

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	items, err := h.health.Compute(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	// Optional filter by category
	cat := r.URL.Query().Get("cat")
	if cat != "" {
		var filtered []SongHealth
		for _, it := range items {
			if it.Category == cat {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	counts := map[string]int{}
	all, _ := h.health.Compute(r.Context())
	for _, it := range all {
		counts[it.Category]++
	}
	counts["all"] = len(all)

	h.renderer.Render(w, r, http.StatusOK, "songs_health", map[string]any{
		"Songs":  items,
		"Counts": counts,
		"Filter": cat,
	})
}

// ---- helpers ----

// ---- Translation handlers ----

func (h *Handler) NewTranslationForm(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := h.arr.Get(r.Context(), arrID)
	if err != nil || a.SongID != songID {
		http.NotFound(w, r)
		return
	}
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if lang == "" {
		lang = "fr"
	}

	// If a translation already exists, pre-fill the form.
	var existing ArrangementTranslation
	if t, err := h.arr.GetTranslation(r.Context(), arrID, lang); err == nil {
		existing = t
	}

	h.renderTranslationForm(w, r, song, a, lang, existing.SourceContent, existing.Format, existing.Notes, "")
}

func (h *Handler) SaveTranslation(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}
	lang := r.FormValue("language")
	source := r.FormValue("source_content")
	format := r.FormValue("format")
	notes := r.FormValue("notes")

	if _, err := h.arr.UpsertTranslation(r.Context(), arrID, lang, source, format, notes); err != nil {
		song, _ := h.svc.Get(r.Context(), songID)
		a, _ := h.arr.Get(r.Context(), arrID)
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes, err.Error())
		return
	}
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID+"?lang="+lang, http.StatusSeeOther)
}

func (h *Handler) DeleteTranslation(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	_ = h.arr.DeleteTranslation(r.Context(), chi.URLParam(r, "tid"))
	http.Redirect(w, r, "/songs/"+songID+"/arrangements/"+arrID, http.StatusSeeOther)
}

// TranslateSource is the target of a formaction button. It translates the
// arrangement's primary source into the requested language and re-renders
// the translation form with the result pre-filled.
func (h *Handler) TranslateSource(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "id")
	arrID := chi.URLParam(r, "aid")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}
	lang := r.FormValue("language")
	if lang == "" {
		lang = r.URL.Query().Get("lang")
	}

	song, err := h.svc.Get(r.Context(), songID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := h.arr.Get(r.Context(), arrID)
	if err != nil || a.SongID != songID {
		http.NotFound(w, r)
		return
	}

	// Track what to display in the form even on error
	source := r.FormValue("source_content")
	format := r.FormValue("format")
	notes := r.FormValue("notes")

	if !h.trans.Enabled() {
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes,
			"Translation service not configured. Set TRANSLATE_API_URL to enable. You can still paste the translation manually.")
		return
	}

	plain := StripChordsForTranslation(a.SourceContent, a.Format)
	if plain == "" {
		plain = a.Lyrics
	}
	if plain == "" {
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes,
			"No source content to translate.")
		return
	}

	translated, err := h.trans.Translate(r.Context(), plain, a.PrimaryLanguage, lang)
	if err != nil {
		h.renderTranslationForm(w, r, song, a, lang, source, format, notes,
			"Translate failed: "+err.Error())
		return
	}

	h.renderTranslationForm(w, r, song, a, lang, translated, format, notes, "")
}

func (h *Handler) renderTranslationForm(w http.ResponseWriter, r *http.Request, song Song, a Arrangement, lang, source, format, notes, errMsg string) {
	if format == "" {
		format = a.Format
		if format == "" {
			format = "chordpro"
		}
	}
	h.renderer.Render(w, r, http.StatusOK, "songs_translation_form", map[string]any{
		"Song":             song,
		"Arrangement":      a,
		"Language":         lang,
		"SourceContent":    source,
		"Format":           format,
		"Notes":            notes,
		"Error":            errMsg,
		"TranslatorEnabled": h.trans != nil && h.trans.Enabled(),
	})
}

// ---- Suggestions ----

func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tagSlugs := q["tag"]
	key := strings.TrimSpace(q.Get("key"))
	limit := 15
	if n, _ := strconv.Atoi(q.Get("limit")); n > 0 {
		limit = n
	}

	var suggestions []Suggestion
	var err error
	run := r.URL.Query().Get("run") == "1" || len(tagSlugs) > 0 || key != ""
	if run {
		suggestions, err = h.sugg.Suggest(r.Context(), tagSlugs, key, limit)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
	}

	groups, err := h.tags.ListGrouped(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	selected := map[string]bool{}
	for _, s := range tagSlugs {
		selected[s] = true
	}

	h.renderer.Render(w, r, http.StatusOK, "songs_suggestions", map[string]any{
		"Songs":            suggestions,
		"TagGroups":        groups,
		"SelectedTagSlugs": selected,
		"Key":              key,
		"Limit":            limit,
		"Ran":              run,
	})
}

// SearchArrangementsJSON returns (song, arrangement) pairs as JSON.
func (h *Handler) SearchArrangementsJSON(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, err := h.arr.SearchArrangements(r.Context(), q, 60)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if results == nil {
		results = []ArrangementSearchResult{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// youtubeID extracts the video ID from common YouTube URL forms.
// Returns "" if the URL isn't recognised (caller falls back to a plain link).
func youtubeID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Normalise scheme + www.
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")

	// youtu.be/ID
	if strings.HasPrefix(s, "youtu.be/") {
		id := strings.TrimPrefix(s, "youtu.be/")
		if i := strings.IndexAny(id, "?&#/"); i >= 0 {
			id = id[:i]
		}
		return id
	}
	// youtube.com/{shorts,embed,v}/ID
	for _, prefix := range []string{
		"youtube.com/shorts/",
		"youtube.com/embed/",
		"youtube.com/v/",
	} {
		if strings.HasPrefix(s, prefix) {
			id := strings.TrimPrefix(s, prefix)
			if i := strings.IndexAny(id, "?&#/"); i >= 0 {
				id = id[:i]
			}
			return id
		}
	}
	// youtube.com/watch?v=ID (query may contain other params)
	if strings.HasPrefix(s, "youtube.com/watch") {
		if i := strings.Index(s, "v="); i >= 0 {
			id := s[i+2:]
			if j := strings.IndexAny(id, "?&#"); j >= 0 {
				id = id[:j]
			}
			return id
		}
	}
	return ""
}

func (h *Handler) publicBaseURL(r *http.Request) string {
	if h.cfg.PublicBaseURL != "" {
		return strings.TrimRight(h.cfg.PublicBaseURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
