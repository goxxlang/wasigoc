// Tiny subset of container/ring: circular list.
package ring

type Ring struct {
	next  *Ring
	prev  *Ring
	Value any
}

func New(n int) *Ring {
	if n <= 0 {
		return nil
	}
	r := &Ring{}
	p := r
	for i := 1; i < n; i++ {
		p.next = &Ring{prev: p}
		p = p.next
	}
	p.next = r
	r.prev = p
	return r
}

func (r *Ring) Next() *Ring {
	return r.next
}

func (r *Ring) Prev() *Ring {
	return r.prev
}

func (r *Ring) Len() int {
	n := 0
	if r != nil {
		n = 1
		for p := r.next; p != r; p = p.next {
			n++
		}
	}
	return n
}

func (r *Ring) Do(f func(any)) {
	if r == nil {
		return
	}
	f(r.Value)
	for p := r.next; p != r; p = p.next {
		f(p.Value)
	}
}

func (r *Ring) Move(n int) *Ring {
	if r == nil {
		return nil
	}
	p := r
	if n < 0 {
		for ; n < 0; n++ {
			p = p.prev
		}
	} else {
		for ; n > 0; n-- {
			p = p.next
		}
	}
	return p
}

func (r *Ring) Link(s *Ring) *Ring {
	n := r.next
	if s != nil {
		p := s.prev
		r.next = s
		s.prev = r
		n.prev = p
		p.next = n
	}
	return n
}

func (r *Ring) Unlink(n int) *Ring {
	if n <= 0 {
		return nil
	}
	return r.Link(r.Move(n + 1))
}
