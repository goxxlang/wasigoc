// Tiny subset of container/list: doubly linked list of any.
// (Nil-terminated head/tail, not Go's circular-sentinel-ring internals --
// the public Element/List API behaves the same.)
package list

type Element struct {
	next  *Element
	prev  *Element
	list  *List
	Value any
}

func (e *Element) Next() *Element {
	return e.next
}

func (e *Element) Prev() *Element {
	return e.prev
}

type List struct {
	head *Element
	tail *Element
	n    int
}

func New() *List {
	l := &List{}
	return l
}

func (l *List) Len() int {
	return l.n
}

func (l *List) Front() *Element {
	return l.head
}

func (l *List) Back() *Element {
	return l.tail
}

func (l *List) PushBack(v any) *Element {
	e := &Element{Value: v, list: l}
	if l.tail == nil {
		l.head = e
		l.tail = e
	} else {
		e.prev = l.tail
		l.tail.next = e
		l.tail = e
	}
	l.n++
	return e
}

func (l *List) PushFront(v any) *Element {
	e := &Element{Value: v, list: l}
	if l.head == nil {
		l.head = e
		l.tail = e
	} else {
		e.next = l.head
		l.head.prev = e
		l.head = e
	}
	l.n++
	return e
}

func (l *List) Remove(e *Element) any {
	if e.list != l {
		return e.Value
	}
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		l.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		l.tail = e.prev
	}
	e.next = nil
	e.prev = nil
	e.list = nil
	l.n--
	return e.Value
}

func (l *List) InsertBefore(v any, mark *Element) *Element {
	if mark == nil || mark.list != l {
		return nil
	}
	e := &Element{Value: v, list: l}
	e.prev = mark.prev
	e.next = mark
	if mark.prev != nil {
		mark.prev.next = e
	} else {
		l.head = e
	}
	mark.prev = e
	l.n++
	return e
}

func (l *List) InsertAfter(v any, mark *Element) *Element {
	if mark == nil || mark.list != l {
		return nil
	}
	e := &Element{Value: v, list: l}
	e.next = mark.next
	e.prev = mark
	if mark.next != nil {
		mark.next.prev = e
	} else {
		l.tail = e
	}
	mark.next = e
	l.n++
	return e
}

func (l *List) Init() *List {
	l.head = nil
	l.tail = nil
	l.n = 0
	return l
}
