package site

import "strconv"

// paginate splits list and taxonomy pages into numbered pages:
// /articles/, /articles/page/2/, ... Every list page gets a Paginator, so a
// template can range over .Paginator.Items whether or not the list is split.
func (s *Site) paginate() {
	var extra []*Page
	for _, p := range s.Pages {
		if p.Kind != KindList && p.Kind != KindTerm {
			continue
		}
		per := s.Config.Paginate
		if p.Kind == KindList {
			if n := s.Config.Collections[p.Collection].Paginate; n > 0 {
				per = n
			}
		}
		items := p.Pages
		if per <= 0 || len(items) <= per {
			p.Paginator = &Paginator{Number: 1, TotalPages: 1, TotalItems: len(items), Items: items, First: p}
			continue
		}
		total := (len(items) + per - 1) / per
		pages := make([]*Page, total)
		pages[0] = p
		for i := 1; i < total; i++ {
			cp := *p
			cp.Source = ""
			cp.Content = ""
			cp.Path = p.Path + "page/" + strconv.Itoa(i+1) + "/"
			cp.Permalink = s.Config.URL + cp.Path
			pages[i] = &cp
			extra = append(extra, &cp)
		}
		for i, pg := range pages {
			end := (i + 1) * per
			if end > len(items) {
				end = len(items)
			}
			pg.Paginator = &Paginator{Number: i + 1, TotalPages: total, TotalItems: len(items), Items: items[i*per : end], First: p}
		}
		for i, pg := range pages {
			if i > 0 {
				pg.Paginator.Prev = pages[i-1]
			}
			if i < total-1 {
				pg.Paginator.Next = pages[i+1]
			}
		}
	}
	s.Pages = append(s.Pages, extra...)
}
