package gui

import (
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"golang.org/x/sync/errgroup"
)

// sort codes handled by the gui server; the catalog api has no launch time order
// (its "newest" order is not strictly by launch and has no ascending variant)
const (
	sortLaunchNewest = 101
	sortLaunchOldest = 102

	catalogCollectSize = 30 // the catalog api misbehaves with larger pages
	catalogCollectMax  = 60 // pages
	catalogAllTTL      = 10 * time.Minute
)

func isLaunchSort(v int) bool { return v == sortLaunchNewest || v == sortLaunchOldest }

type cachedCatalog struct {
	items []productDTO
	at    time.Time
}

// catalogAll caches the whole result list of a filter for client side sorting
type catalogAll struct {
	mu      sync.Mutex
	entries map[string]cachedCatalog
	// one collection per filter at a time
	inflight map[string]*sync.Mutex
}

func catalogKey(q geektime.CatalogQuery) string {
	return strconv.Itoa(q.TagID) + "/" + strconv.Itoa(q.ProductType) + "/" + strconv.Itoa(q.ProductForm) + "/" + strconv.Itoa(q.Pvip)
}

func (c *catalogAll) lock(key string) func() {
	c.mu.Lock()
	if c.inflight == nil {
		c.inflight = make(map[string]*sync.Mutex)
	}
	m, ok := c.inflight[key]
	if !ok {
		m = &sync.Mutex{}
		c.inflight[key] = m
	}
	c.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func (c *catalogAll) get(key string) ([]productDTO, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Since(e.at) > catalogAllTTL {
		return nil, false
	}
	return e.items, true
}

func (c *catalogAll) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

func (c *catalogAll) put(key string, items []productDTO) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]cachedCatalog)
	}
	c.entries[key] = cachedCatalog{items, time.Now()}
}

// collectCatalog returns every course matching the filters of q (sort and page ignored)
func (s *Server) collectCatalog(client *geektime.Client, q geektime.CatalogQuery) ([]productDTO, error) {
	key := catalogKey(q)
	if items, ok := s.catalogAll.get(key); ok {
		return items, nil
	}
	unlock := s.catalogAll.lock(key)
	defer unlock()
	if items, ok := s.catalogAll.get(key); ok {
		return items, nil
	}

	fetch := func(page int) (geektime.CatalogPage, error) {
		pq := q
		pq.Sort, pq.Page, pq.Size = 8, page, catalogCollectSize
		return client.ProductList(pq)
	}
	first, err := fetch(1)
	if err != nil {
		return nil, err
	}
	pages := [][]productDTO{toDTOs(first, 1)}
	if first.More {
		// the reported total includes delisted courses, so some trailing pages may be empty
		n := (first.Total + catalogCollectSize - 1) / catalogCollectSize
		if n > catalogCollectMax {
			n = catalogCollectMax
		}
		if n < 2 {
			n = 2
		}
		rest := make([][]productDTO, n-1)
		g := errgroup.Group{}
		g.SetLimit(4)
		for page := 2; page <= n; page++ {
			g.Go(func() error {
				res, err := fetch(page)
				if err != nil {
					return err
				}
				rest[page-2] = toDTOs(res, page)
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
		pages = append(pages, rest...)
	}

	var items []productDTO
	seen := make(map[int]bool)
	for _, list := range pages {
		for _, p := range list {
			if !seen[p.ID] {
				seen[p.ID] = true
				items = append(items, p)
			}
		}
	}
	s.catalogAll.put(key, items)
	return items, nil
}

func toDTOs(res geektime.CatalogPage, page int) []productDTO {
	out := make([]productDTO, 0, len(res.Products))
	for i, p := range res.Products {
		out = append(out, toProductDTO(p, (page-1)*catalogCollectSize+i))
	}
	return out
}

// sortByLaunch orders a copy of items by launch time; unknown launch times go last
func sortByLaunch(items []productDTO, newest bool) []productDTO {
	out := append([]productDTO(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].BeginTime, out[j].BeginTime
		if (a == 0) != (b == 0) {
			return b == 0
		}
		if newest {
			return a > b
		}
		return a < b
	})
	return out
}
