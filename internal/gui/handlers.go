package gui

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/config"
	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
	"github.com/nicoxiang/geektime-downloader/internal/ui"
)

// productTypeDTO is one product type, key "n<index>" for normal, "e<index>" for enterprise
type productTypeDTO struct {
	Key               string `json:"key"`
	Name              string `json:"name"`
	NeedSelectArticle bool   `json:"needSelectArticle"`
	Enterprise        bool   `json:"enterprise"`
}

// autoOption is used for courses opened from lists whose exact type code is not one of the
// CLI product types (e.g. x50, c6): the course is loaded with CourseInfo without type validation.
var autoOption = ui.ProductTypeSelectOption{
	Index:             autoIndex,
	Text:              "课程",
	SourceType:        1,
	NeedSelectArticle: true,
}

const (
	autoIndex = 99
	autoKey   = "a0"
)

func productTypes() []productTypeDTO {
	out := []productTypeDTO{{autoKey, "自动识别", true, false}}
	for _, o := range ui.ProductTypeOptions(false) {
		out = append(out, productTypeDTO{"n" + strconv.Itoa(o.Index), o.Text, o.NeedSelectArticle, false})
	}
	for _, o := range ui.ProductTypeOptions(true) {
		out = append(out, productTypeDTO{"e" + strconv.Itoa(o.Index), "企业版" + o.Text, o.NeedSelectArticle, true})
	}
	return out
}

func productTypeByKey(key string) (ui.ProductTypeSelectOption, bool) {
	if key == autoKey {
		return autoOption, true
	}
	if len(key) < 2 {
		return ui.ProductTypeSelectOption{}, false
	}
	idx, err := strconv.Atoi(key[1:])
	if err != nil {
		return ui.ProductTypeSelectOption{}, false
	}
	var opts []ui.ProductTypeSelectOption
	switch key[0] {
	case 'n':
		opts = ui.ProductTypeOptions(false)
	case 'e':
		opts = ui.ProductTypeOptions(true)
	default:
		return ui.ProductTypeSelectOption{}, false
	}
	for _, o := range opts {
		if o.Index == idx {
			return o, true
		}
	}
	return ui.ProductTypeSelectOption{}, false
}

func typeKeyOf(o ui.ProductTypeSelectOption) string {
	if o.Index == autoIndex {
		return autoKey
	}
	if o.IsEnterpriseMode {
		return "e" + strconv.Itoa(o.Index)
	}
	return "n" + strconv.Itoa(o.Index)
}

type productDTO struct {
	ID           int    `json:"id"`       // product id
	CourseID     int    `json:"courseId"` // id used to open/download, class id for university
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
	Author       string `json:"author"`
	AuthorIntro  string `json:"authorIntro"`
	Cover        string `json:"cover"`
	Code         string `json:"code"`
	TypeKey      string `json:"typeKey"`
	TypeName     string `json:"typeName"`
	IsVideo      bool   `json:"isVideo"`
	IsFinish     bool   `json:"isFinish"`
	ArticleCount int    `json:"articleCount"`
	Unit         string `json:"unit"`
	Direct       bool   `json:"direct"`    // no article selection, download as one video
	Openable     bool   `json:"openable"`  // can be opened/downloaded by this tool
	Purchased    bool   `json:"purchased"` // current user can learn it
	Local        bool   `json:"local"`     // a non-empty course folder exists in the download folder
	Claimable    bool   `json:"claimable"` // free open course that can be unlocked ("免费领取")
	Order        int    `json:"order"`
}

func toProductDTO(p response.LearnProduct, order int) productDTO {
	dto := productDTO{
		ID:          p.ID,
		CourseID:    p.ID,
		Title:       p.Title,
		Subtitle:    p.Subtitle,
		Author:      p.Author.Name,
		AuthorIntro: p.Author.Intro,
		Cover:       p.Cover.Square,
		Code:        p.Type,
		IsVideo:     p.IsVideo,
		IsFinish:    p.IsFinish,
		Unit:        p.Unit,
		Purchased:   p.Extra.Sub.HadDone,
		Order:       order,
	}
	if dto.Cover == "" {
		dto.Cover = p.Cover.Rectangle
	}
	if n, err := p.Article.Count.Int64(); err == nil {
		dto.ArticleCount = int(n)
	}
	classifyProduct(&dto, p.IsUniversity, p.University.ClassID)
	dto.Claimable = !dto.Purchased && p.Price.SaleType == geektime.SaleTypeFreeClaim
	return dto
}

// classifyProduct decides how a product is opened and downloaded
func classifyProduct(dto *productDTO, isUniversity bool, classID int) {
	switch {
	case isUniversity || strings.HasPrefix(dto.Code, "u"):
		// university products are opened by class id on u.geekbang.org
		dto.TypeName = "训练营"
		if classID > 0 {
			o, _ := productTypeByKey("n4")
			dto.TypeKey = typeKeyOf(o)
			dto.CourseID = classID
			dto.Openable = true
		}
	default:
		if o, ok := ui.ProductTypeOptionByCode(dto.Code); ok {
			dto.TypeKey = typeKeyOf(o)
			dto.TypeName = o.Text
			dto.Direct = !o.NeedSelectArticle
		} else {
			dto.TypeKey = autoKey
			dto.TypeName = "其他"
		}
		dto.Openable = true
	}
}

type articleDTO struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Section string `json:"section"`
}

func articleSection(a geektime.Article) string {
	if a.SectionTitle != "" {
		return a.SectionTitle
	}
	return a.Chapter
}

type courseDTO struct {
	ID         int          `json:"id"`
	Title      string       `json:"title"`
	IsVideo    bool         `json:"isVideo"`
	TypeKey    string       `json:"typeKey"`
	TypeName   string       `json:"typeName"`
	Articles   []articleDTO `json:"articles"`
	Downloaded []int        `json:"downloaded"` // article ids recorded as downloaded
	Marked     bool         `json:"marked"`     // manually marked as downloaded
	Local      bool         `json:"local"`
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	_, st := s.snapshotConfig()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"loggedIn": s.currentClient() != nil,
		"settings": st,
		"types":    productTypes(),
		"limits": map[string]int{
			"maxThreads":         config.MaxThreads,
			"maxConcurrentTasks": maxConcurrentTasks,
		},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Gcid  string `json:"gcid"`
		Gcess string `json:"gcess"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Gcid, req.Gcess = strings.TrimSpace(req.Gcid), strings.TrimSpace(req.Gcess)
	if req.Gcid == "" || req.Gcess == "" {
		writeError(w, http.StatusBadRequest, "GCID 和 GCESS 不能为空")
		return
	}

	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	cfg.Gcid, cfg.Gcess = req.Gcid, req.Gcess
	cookies := config.ReadCookiesFromInput(&cfg)
	if err := geektime.Auth(cookies); err != nil {
		writeError(w, http.StatusUnauthorized, "登录校验失败: "+err.Error())
		return
	}

	s.mu.Lock()
	s.cfg.Gcid, s.cfg.Gcess = req.Gcid, req.Gcess
	s.client = geektime.NewClient(cookies)
	s.mu.Unlock()
	s.clearCache()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.tasks.cancelAll()
	s.mu.Lock()
	s.cfg.Gcid, s.cfg.Gcess = "", ""
	s.client = nil
	s.mu.Unlock()
	s.clearCache()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) clearCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.products = nil
	s.catalogMeta = nil
	s.introCache = make(map[string]introDTO)
	s.courseCache = make(map[string]geektime.Course)
}

func (s *Server) requireClient(w http.ResponseWriter) *geektime.Client {
	c := s.currentClient()
	if c == nil {
		writeError(w, http.StatusUnauthorized, "请先登录")
	}
	return c
}

func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"

	s.cacheMu.Lock()
	cached := s.products
	s.cacheMu.Unlock()
	if cached != nil && !refresh {
		writeJSON(w, http.StatusOK, map[string]interface{}{"products": s.withLocal(cached)})
		return
	}

	list, err := client.MyProducts()
	if err != nil {
		writeAPIError(w, err)
		return
	}
	products := make([]productDTO, 0, len(list))
	for i, p := range list {
		dto := toProductDTO(p, i)
		dto.Purchased = true
		products = append(products, dto)
	}
	s.cacheMu.Lock()
	s.products = products
	s.cacheMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"products": s.withLocal(products)})
}

func (s *Server) handleCourse(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	typeKey := r.URL.Query().Get("type")
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "课程 ID 格式不合法")
		return
	}
	option, ok := productTypeByKey(typeKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "未知的课程类型")
		return
	}
	if !option.NeedSelectArticle {
		writeError(w, http.StatusBadRequest, option.Text+"无需选择文章, 请直接下载")
		return
	}

	c, status, err := s.loadCourse(client, option, id, r.URL.Query().Get("refresh") == "1")
	if err != nil {
		if status != 0 {
			writeError(w, status, err.Error())
		} else {
			writeAPIError(w, err)
		}
		return
	}

	typeName := option.Text
	if option.Index == autoIndex {
		if actual, ok := ui.ProductTypeOptionByCode(c.Type); ok {
			typeName = actual.Text
		}
	}
	dto := courseDTO{
		ID:       c.ID,
		Title:    c.Title,
		IsVideo:  c.IsVideo,
		TypeKey:  typeKey,
		TypeName: typeName,
		Articles: make([]articleDTO, 0, len(c.Articles)),
	}
	for _, a := range c.Articles {
		dto.Articles = append(dto.Articles, articleDTO{a.AID, a.Title, articleSection(a)})
	}
	s.records.setTotal(c.ID, c.Title, len(c.Articles), false)
	dto.Downloaded, dto.Marked = s.records.articles(c.ID)
	dto.Local = s.localExists(c.Title)
	writeJSON(w, http.StatusOK, dto)
}

// loadCourse fetches (or returns cached) course info. A non-zero status means a user facing error.
func (s *Server) loadCourse(client *geektime.Client, option ui.ProductTypeSelectOption, id int, refresh bool) (geektime.Course, int, error) {
	key := typeKeyOf(option) + ":" + strconv.Itoa(id)
	if !refresh {
		s.cacheMu.Lock()
		c, ok := s.courseCache[key]
		s.cacheMu.Unlock()
		if ok {
			return c, 0, nil
		}
	}

	var c geektime.Course
	var err error
	switch {
	case option.IsEnterpriseMode:
		c, err = client.EnterpriseCourseInfo(id)
	case option.IsUniversity():
		c, err = client.UniversityClassInfo(id)
	case option.Index == autoIndex:
		c, err = client.CourseInfo(id)
		if err == nil {
			if actual, ok := ui.ProductTypeOptionByCode(c.Type); ok && !actual.NeedSelectArticle {
				return c, http.StatusBadRequest, fmt.Errorf("该 ID 属于「%s」, 请选择对应的类型后直接下载", actual.Text)
			}
			if strings.HasPrefix(c.Type, "u") {
				return c, http.StatusBadRequest, errors.New("训练营请从「我的课程」中打开, 或选择「训练营」类型并输入班级 ID")
			}
			if c.Title == "" {
				return c, http.StatusNotFound, errors.New("课程不存在, 请检查课程 ID")
			}
		}
	default:
		c, err = client.CourseInfo(id)
		if err == nil && !option.Accepts(c.Type) {
			if actual, ok := ui.ProductTypeOptionByCode(c.Type); ok {
				return c, http.StatusBadRequest, fmt.Errorf("课程 ID 与类型不匹配, 该课程属于「%s」", actual.Text)
			}
			return c, http.StatusBadRequest, errors.New("输入的课程 ID 有误")
		}
	}
	if err != nil {
		return c, 0, err
	}
	if !c.Access {
		return c, http.StatusForbidden, errors.New("尚未购买该课程")
	}

	s.cacheMu.Lock()
	s.courseCache[key] = c
	s.cacheMu.Unlock()
	return c, 0, nil
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": s.tasks.list()})
}

func (s *Server) handleCreateTasks(w http.ResponseWriter, r *http.Request) {
	client := s.requireClient(w)
	if client == nil {
		return
	}
	var req struct {
		Type       string `json:"type"`
		CourseID   int    `json:"courseId"`
		ArticleIDs []int  `json:"articleIds"`
		Overwrite  bool   `json:"overwrite"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	option, ok := productTypeByKey(req.Type)
	if !ok {
		writeError(w, http.StatusBadRequest, "未知的课程类型")
		return
	}
	if req.CourseID <= 0 {
		writeError(w, http.StatusBadRequest, "课程 ID 格式不合法")
		return
	}

	if !option.NeedSelectArticle {
		s.createProductTask(w, client, option, req.Type, req.CourseID, req.Overwrite)
		return
	}

	c, status, err := s.loadCourse(client, option, req.CourseID, false)
	if err != nil {
		if status != 0 {
			writeError(w, status, err.Error())
		} else {
			writeAPIError(w, err)
		}
		return
	}

	want := make(map[int]bool, len(req.ArticleIDs))
	for _, id := range req.ArticleIDs {
		want[id] = true
	}
	var tasks []*Task
	for _, a := range c.Articles {
		if len(want) > 0 && !want[a.AID] {
			continue
		}
		tasks = append(tasks, &Task{
			CourseID:    c.ID,
			CourseTitle: c.Title,
			ArticleID:   a.AID,
			Title:       a.Title,
			TypeKey:     req.Type,
			TypeName:    option.Text,
			IsVideo:     c.IsVideo,
			kind:        kindArticle,
			course:      c,
			option:      option,
			article:     a,
			overwrite:   req.Overwrite,
		})
	}
	s.records.setTotal(c.ID, c.Title, len(c.Articles), true)
	if len(tasks) == 0 {
		writeError(w, http.StatusBadRequest, "没有可下载的文章")
		return
	}
	added := s.tasks.add(tasks)
	writeJSON(w, http.StatusOK, map[string]int{"added": added, "requested": len(tasks)})
}

// createProductTask handles 每日一课 / 大厂案例: the id is a product id and it contains one video
func (s *Server) createProductTask(w http.ResponseWriter, client *geektime.Client, option ui.ProductTypeSelectOption, typeKey string, id int, overwrite bool) {
	info, err := client.ProductInfo(id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if !option.Accepts(info.Data.Info.Type) {
		writeError(w, http.StatusBadRequest, "输入的课程 ID 有误")
		return
	}
	if info.Data.Info.Extra.Sub.AccessMask == 0 {
		writeError(w, http.StatusForbidden, "尚未购买该课程")
		return
	}
	title := info.Data.Info.Title
	added := s.tasks.add([]*Task{{
		CourseID:    id,
		CourseTitle: title,
		ArticleID:   info.Data.Info.Article.ID,
		Title:       title,
		TypeKey:     typeKey,
		TypeName:    option.Text,
		IsVideo:     true,
		kind:        kindProduct,
		option:      option,
		overwrite:   overwrite,
	}})
	s.records.setTotal(id, title, 1, true)
	writeJSON(w, http.StatusOK, map[string]int{"added": added, "requested": 1})
}

func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string  `json:"action"`
		IDs    []int64 `json:"ids"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	switch req.Action {
	case "cancel":
		s.tasks.cancel(req.IDs)
	case "retry":
		s.tasks.retry(req.IDs)
	case "remove":
		s.tasks.remove(req.IDs)
	case "cancelAll":
		s.tasks.cancelAll()
	case "retryFailed":
		s.tasks.retryFailed()
	case "clearFinished":
		s.tasks.clearFinished()
	default:
		writeError(w, http.StatusBadRequest, "未知操作")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": s.tasks.list()})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	_, st := s.snapshotConfig()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var st Settings
	if err := readJSON(w, r, &st); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	s.mu.Lock()
	err := st.validate(s.cfg)
	if err == nil {
		s.settings = st
	}
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// concurrency may have been raised
	s.tasks.reschedule()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID int64 `json:"taskId"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	cfg, _ := s.snapshotConfig()
	dir := cfg.DownloadFolder
	if req.TaskID > 0 {
		if d, ok := s.tasks.taskDir(req.TaskID); ok && d != "" {
			if _, err := os.Stat(d); err == nil {
				dir = d
			}
		}
	}
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := openPath(dir); err != nil {
		writeError(w, http.StatusInternalServerError, "打开目录失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
}

// statusOf maps an api error to the http status writeAPIError would use
func statusOf(err error) int {
	switch {
	case errors.Is(err, geektime.ErrAuthFailed):
		return http.StatusUnauthorized
	case errors.Is(err, geektime.ErrGeekTimeRateLimit):
		return http.StatusTooManyRequests
	}
	return 0
}

func writeAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, geektime.ErrAuthFailed):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, geektime.ErrGeekTimeRateLimit):
		writeError(w, http.StatusTooManyRequests, err.Error())
	case os.IsTimeout(err):
		writeError(w, http.StatusGatewayTimeout, "请求超时")
	default:
		writeError(w, http.StatusBadGateway, err.Error())
	}
}
